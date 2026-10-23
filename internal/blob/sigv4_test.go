package blob

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// Vectors from the AWS Signature Version 4 test suite (aws-sig-v4-test-suite,
// as shipped in botocore tests/unit/auth/aws4_testsuite): service "service",
// region us-east-1, 2015-08-30T12:36:00Z, key AKIDEXAMPLE.
func TestSigV4TestSuite(t *testing.T) {
	s := signer{AccessKey: "AKIDEXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", Region: "us-east-1", Service: "service"}
	when := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	cases := []struct {
		name, method, target, body string
		headers                    [][2]string
		creq, sts, authz           string
	}{
		{
			name: "get-vanilla", method: "GET", target: "/",
			creq: "GET\n/\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			sts:  "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\nbb579772317eb040ac9ed261061d46c1f17a8133879d6129b6e1c25292927e63",
			authz: "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, " +
				"Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31",
		},
		{
			name: "get-vanilla-query-order-key-case", method: "GET", target: "/?Param2=value2&Param1=value1",
			authz: "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, " +
				"Signature=b97d918cfa904a5beff61c982a1b6f458b799221646efd99d3219ec94cdf2500",
		},
		{
			name: "get-vanilla-empty-query-key", method: "GET", target: "/?Param1=value1",
			authz: "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, " +
				"Signature=a67d582fa61cc504c4bae71f336f98b97f1ea3c7a6bfe1b6e45aec72011b9aeb",
		},
		{
			name: "get-vanilla-utf8-query", method: "GET", target: "/?%E1%88%B4=bar",
			authz: "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, " +
				"Signature=2cdec8eed098649ff3a119c94853b13c643bcf08f8b0a1d91e12c9027818dd04",
		},
		{
			name: "get-header-value-trim", method: "GET", target: "/",
			headers: [][2]string{{"My-Header1", " value1"}, {"My-Header2", `"a   b   c"`}},
			authz: "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;my-header1;my-header2;x-amz-date, " +
				"Signature=acc3ed3afb60bb290fc8d2dd0098b9911fcaa05412b367055dee359757a9c736",
		},
		{
			name: "post-vanilla", method: "POST", target: "/",
			authz: "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, " +
				"Signature=5da7c1a2acd57cee7505fc6676e4e544621c30862966e37dddb68e92efbe5d6b",
		},
		{
			name: "post-x-www-form-urlencoded", method: "POST", target: "/", body: "Param1=value1",
			headers: [][2]string{{"Content-Type", "application/x-www-form-urlencoded"}},
			authz: "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=content-type;host;x-amz-date, " +
				"Signature=ff11897932ad3f4e8b18135d722051e5ac45fc38421b1da7b9d196a0fe09473a",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, err := http.NewRequest(c.method, "https://example.amazonaws.com"+c.target, strings.NewReader(c.body))
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range c.headers {
				req.Header.Add(h[0], h[1])
			}
			hash := sha256Hex([]byte(c.body))
			s.sign(req, hash, when)
			if c.creq != "" || c.sts != "" {
				creq, _ := canonicalRequest(req, hash)
				if c.creq != "" && creq != c.creq {
					t.Errorf("canonical request:\n%s\nwant:\n%s", creq, c.creq)
				}
				sts := stringToSign("20150830T123600Z", "20150830/us-east-1/service/aws4_request", creq)
				if c.sts != "" && sts != c.sts {
					t.Errorf("string to sign:\n%s\nwant:\n%s", sts, c.sts)
				}
			}
			if got := req.Header.Get("Authorization"); got != c.authz {
				t.Errorf("authorization:\n%s\nwant:\n%s", got, c.authz)
			}
		})
	}
}

// Examples from the Amazon S3 API reference ("Signature calculations for
// the Authorization header", examples GET Object and PUT Object), which sign
// x-amz-content-sha256 like the S3 store does.
func TestSigV4S3Examples(t *testing.T) {
	s := signer{AccessKey: "AKIAIOSFODNN7EXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", Region: "us-east-1", Service: "s3"}
	when := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

	get, _ := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	get.Header.Set("Range", "bytes=0-9")
	get.Header.Set("X-Amz-Content-Sha256", sha256Hex(nil))
	s.sign(get, sha256Hex(nil), when)
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := get.Header.Get("Authorization"); got != want {
		t.Errorf("GET Object:\n%s\nwant:\n%s", got, want)
	}

	body := "Welcome to Amazon S3."
	put, _ := http.NewRequest("PUT", "https://examplebucket.s3.amazonaws.com/test$file.text", strings.NewReader(body))
	put.Header.Set("Date", "Fri, 24 May 2013 00:00:00 GMT")
	put.Header.Set("X-Amz-Storage-Class", "REDUCED_REDUNDANCY")
	put.Header.Set("X-Amz-Content-Sha256", sha256Hex([]byte(body)))
	put.URL.RawPath = "/test%24file.text"
	s.sign(put, sha256Hex([]byte(body)), when)
	want = "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=date;host;x-amz-content-sha256;x-amz-date;x-amz-storage-class, " +
		"Signature=98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"
	if got := put.Header.Get("Authorization"); got != want {
		t.Errorf("PUT Object:\n%s\nwant:\n%s", got, want)
	}
}

func TestURIEncode(t *testing.T) {
	for in, want := range map[string]string{
		"raw/org/2026/10/a-b_c.~x.eml": "raw/org/2026/10/a-b_c.~x.eml",
		"att/o/m/report final.pdf":     "att/o/m/report%20final.pdf",
		"raw/o/<id@host>.eml":          "raw/o/%3Cid%40host%3E.eml",
		"ä+=":                          "%C3%A4%2B%3D",
	} {
		if got := uriEncode(in, false); got != want {
			t.Errorf("uriEncode(%q) = %q, want %q", in, got, want)
		}
	}
	if got := uriEncode("a/b", true); got != "a%2Fb" {
		t.Errorf("slash encoding: %q", got)
	}
}

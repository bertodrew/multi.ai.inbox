package mailx

import "testing"

// sampleDSN builds a delivery status notification like Postfix sends for a
// rejected recipient.
func sampleDSN(originalID, rcpt, status string) string {
	return "From: MAILER-DAEMON@mx.ext.test\r\nTo: sales@agents.test\r\nSubject: Undelivered Mail Returned to Sender\r\n" +
		"Message-ID: <dsn-" + status + "@mx.ext.test>\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/report; report-type=delivery-status; boundary=\"r1\"\r\n\r\n" +
		"--r1\r\nContent-Type: text/plain\r\n\r\nI'm sorry to have to inform you that your message could not be delivered.\r\n" +
		"--r1\r\nContent-Type: message/delivery-status\r\n\r\n" +
		"Reporting-MTA: dns; mx.ext.test\r\nArrival-Date: Thu, 22 Oct 2026 10:00:00 +0000\r\n\r\n" +
		"Final-Recipient: rfc822; " + rcpt + "\r\nOriginal-Recipient: rfc822;" + rcpt + "\r\nAction: failed\r\nStatus: " + status + "\r\n" +
		"Diagnostic-Code: smtp; 550 5.1.1 <" + rcpt + ">: Recipient address rejected: User unknown\r\n\r\n" +
		"--r1\r\nContent-Type: text/rfc822-headers\r\n\r\n" +
		"From: sales@agents.test\r\nTo: " + rcpt + "\r\nSubject: Hello\r\nMessage-ID: " + originalID + "\r\n\r\n" +
		"--r1--\r\n"
}

func TestParseDSN(t *testing.T) {
	d := ParseDSN([]byte(sampleDSN("<orig.1@agents.test>", "Ghost@Ext.test", "5.1.1")))
	if d == nil {
		t.Fatal("not recognised as a DSN")
	}
	if d.OriginalMessageID != "<orig.1@agents.test>" {
		t.Fatalf("original id = %q", d.OriginalMessageID)
	}
	if len(d.Failed) != 1 || d.Failed[0].Address != "ghost@ext.test" || d.Failed[0].Status != "5.1.1" {
		t.Fatalf("failed = %+v", d.Failed)
	}

	// A delayed (4.x.x) report is not a hard bounce.
	if d := ParseDSN([]byte(sampleDSN("<orig.2@agents.test>", "slow@ext.test", "4.4.1"))); d == nil || len(d.Failed) != 0 {
		t.Fatalf("temporary status counted as failed: %+v", d)
	}
	if d := ParseDSN([]byte("From: a@b.test\r\nSubject: hi\r\n\r\nhello\r\n")); d != nil {
		t.Fatalf("plain mail parsed as DSN: %+v", d)
	}
}

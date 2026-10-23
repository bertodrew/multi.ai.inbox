package mailx

import "testing"

// sampleARF builds a feedback loop report like the large mailbox providers
// send (RFC 5965): a human readable part, the machine readable report and the
// original message.
func sampleARF(feedbackType, extra, originalID string) string {
	return "From: staff@fbl.ext.test\r\nTo: fbl@agents.test\r\nSubject: complaint about message from 192.0.2.1\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/report; report-type=feedback-report; boundary=\"f1\"\r\n\r\n" +
		"--f1\r\nContent-Type: text/plain\r\n\r\nThis is an email abuse report.\r\n" +
		"--f1\r\nContent-Type: message/feedback-report\r\n\r\n" +
		"Feedback-Type: " + feedbackType + "\r\nUser-Agent: SomeGenerator/1.0\r\nVersion: 1\r\n" +
		"Original-Mail-From: <sales@agents.test>\r\nOriginal-Rcpt-To: <Reader@Ext.test>\r\n" + extra +
		"Arrival-Date: Thu, 22 Oct 2026 10:00:00 +0000\r\nSource-IP: 192.0.2.1\r\n\r\n" +
		"--f1\r\nContent-Type: message/rfc822\r\nContent-Disposition: inline\r\n\r\n" +
		"From: sales@agents.test\r\nTo: reader@ext.test\r\nSubject: Offer\r\nMessage-ID: " + originalID + "\r\n\r\nBuy now.\r\n" +
		"--f1--\r\n"
}

func TestParseARF(t *testing.T) {
	a := ParseARF([]byte(sampleARF("abuse", "", "<orig.1@agents.test>")))
	if a == nil || !a.Complaint() {
		t.Fatalf("not recognised as a complaint: %+v", a)
	}
	if a.OriginalMessageID != "<orig.1@agents.test>" {
		t.Fatalf("original id = %q", a.OriginalMessageID)
	}
	if len(a.OriginalRcptTo) != 1 || a.OriginalRcptTo[0] != "reader@ext.test" {
		t.Fatalf("rcpt = %v", a.OriginalRcptTo)
	}
	// An explicit Original-Message-ID wins over the embedded headers.
	a = ParseARF([]byte(sampleARF("Abuse", "Original-Message-ID: <explicit.2@agents.test>\r\n", "<orig.1@agents.test>")))
	if a == nil || a.FeedbackType != "abuse" || a.OriginalMessageID != "<explicit.2@agents.test>" {
		t.Fatalf("explicit id: %+v", a)
	}
	if a := ParseARF([]byte(sampleARF("not-spam", "", "<orig.1@agents.test>"))); a == nil || a.Complaint() {
		t.Fatalf("not-spam counted as a complaint: %+v", a)
	}
	if a := ParseARF([]byte(sampleDSN("<orig.1@agents.test>", "x@ext.test", "5.1.1"))); a != nil {
		t.Fatalf("DSN parsed as ARF: %+v", a)
	}
	if a := ParseARF([]byte("From: a@b.test\r\nSubject: hi\r\n\r\nhello\r\n")); a != nil {
		t.Fatalf("plain mail parsed as ARF: %+v", a)
	}
}

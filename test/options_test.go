package contract_test

import (
	"strconv"
	"strings"
	"testing"
)

// =============================================================================
// Proposal Options Tests - URL Validation
// =============================================================================

// TestProposalOptionsWithURL tests creating proposal options with URLs.
func TestProposalOptionsWithURL(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing option urls",
		"1",
		"Option A###https://example.com/a;Option B###https://example.com/b",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestProposalOptionsWithoutURL tests creating proposal options without URLs.
func TestProposalOptionsWithoutURL(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing options without urls",
		"1",
		"Option A;Option B;Option C",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestProposalOptionTextTooLong tests that option text exceeding limit is rejected.
func TestProposalOptionTextTooLong(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	longText := strings.Repeat("a", 501) // exceeds 500 char limit
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing long option text",
		"1",
		longText + ";Option B",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "option text exceeds maximum length") {
		t.Fatalf("expected option text length rejection, got %q", res.Ret)
	}
}

// TestProposalOptionURLTooLong tests that option URL exceeding limit is rejected.
func TestProposalOptionURLTooLong(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	longURL := "https://example.com/" + strings.Repeat("a", 481) // exceeds 500 char limit
	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing long option url",
		"1",
		"Option A###" + longURL + ";Option B",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "option URL exceeds maximum length") {
		t.Fatalf("expected option URL length rejection, got %q", res.Ret)
	}
}

// TestProposalOptionEmptyText tests that empty option text is rejected.
func TestProposalOptionEmptyText(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing empty option text",
		"1",
		"###https://example.com;Option B", // empty text with URL
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "option text cannot be empty") {
		t.Fatalf("expected empty option text rejection, got %q", res.Ret)
	}
}

// TestProposalRequiresAtLeastTwoOptions tests that proposals need at least 2 options.
func TestProposalRequiresAtLeastTwoOptions(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing single option",
		"1",
		"Only One Option",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "at least 2 options") {
		t.Fatalf("expected minimum options rejection, got %q", res.Ret)
	}
}

// TestProposalOptionsMixedURLs tests creating options with mixed URL presence.
func TestProposalOptionsMixedURLs(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing mixed urls",
		"1",
		"Option A###https://example.com/a;Option B;Option C###https://example.com/c",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestProposalOptionColonInText tests that colons in option text work correctly.
func TestProposalOptionColonInText(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing colons in text",
		"1",
		"Option: A with colon;Option: B also has colon",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestProposalOptionInvalidURLScheme tests that invalid URL schemes are rejected.
func TestProposalOptionInvalidURLScheme(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing invalid url scheme",
		"1",
		"Option A###ftp://example.com;Option B",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "must start with https://") {
		t.Fatalf("expected URL scheme rejection, got %q", res.Ret)
	}
}

// TestProposalOptionDataURLRejected tests that data: URLs are rejected.
func TestProposalOptionDataURLRejected(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing data url",
		"1",
		"Option A###data:text/html,<script>alert(1)</script>;Option B",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "must start with https://") {
		t.Fatalf("expected data URL rejection, got %q", res.Ret)
	}
}

// TestProposalOptionFileURLRejected tests that file: URLs are rejected.
func TestProposalOptionFileURLRejected(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing file url",
		"1",
		"Option A###file:///etc/passwd;Option B",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "must start with https://") {
		t.Fatalf("expected file URL rejection, got %q", res.Ret)
	}
}

// TestProposalOptionHTTPSAllowed tests that HTTPS URLs are allowed.
func TestProposalOptionHTTPSAllowed(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing https url",
		"1",
		"Option A###https://example.com/path?query=value;Option B###https://another.com",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", true, uint(1_000_000_000))
}

// TestProposalOptionHTTPRejected tests that HTTP (non-HTTPS) URLs are rejected.
func TestProposalOptionHTTPRejected(t *testing.T) {
	ct := SetupContractTest()
	projectID := createDefaultProject(t, ct)

	fields := []string{
		strconv.FormatUint(projectID, 10),
		"test options",
		"testing http url",
		"1",
		"Option A###http://example.com;Option B",
		"1",
		"",
		"",
		"",
	}
	payload := strings.Join(fields, "|")
	res, _, _ := CallContract(t, ct, "proposal_create", PayloadString(payload), transferIntent("1.000"), "hive:someone", false, uint(1_000_000_000))
	if !strings.Contains(res.Ret, "must start with https://") {
		t.Fatalf("expected HTTP URL rejection, got %q", res.Ret)
	}
}

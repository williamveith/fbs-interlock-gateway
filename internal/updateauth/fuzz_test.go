package updateauth

import (
	"encoding/hex"
	"strings"
	"testing"
)

func FuzzParseSignedChecksum(f *testing.F) {
	const lowerSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const upperSHA = "ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789"

	// Valid GNU-style text and binary checksum forms.
	f.Add([]byte(lowerSHA + "  fbs-interlock-gateway-linux-amd64"))
	f.Add([]byte(upperSHA + " *fbs-interlock-gateway-linux-amd64\n"))
	f.Add([]byte("\t" + lowerSHA + "\tasset-name\r\n"))

	// Structural failures.
	f.Add([]byte(""))
	f.Add([]byte("garbage"))
	f.Add([]byte("\n"))
	f.Add([]byte("abc file"))
	f.Add([]byte(lowerSHA))
	f.Add([]byte(lowerSHA + "  first\n" + lowerSHA + "  second"))
	f.Add([]byte(lowerSHA + "  one extra-field"))

	// Digest and filename failures.
	f.Add([]byte(strings.Repeat("a", 63) + "  asset"))
	f.Add([]byte(strings.Repeat("a", 65) + "  asset"))
	f.Add([]byte(strings.Repeat("g", 64) + "  asset"))
	f.Add([]byte(lowerSHA + "  dir/asset"))
	f.Add([]byte(lowerSHA + `  dir\asset`))
	f.Add([]byte(lowerSHA + "  *"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}

		checksum, asset, err := parseSignedChecksum(data)
		if err != nil {
			return
		}

		if len(checksum) != 64 {
			t.Fatalf("accepted checksum with length %d", len(checksum))
		}
		if checksum != strings.ToLower(checksum) {
			t.Fatalf("returned checksum is not normalized to lowercase: %q", checksum)
		}

		decoded, err := hex.DecodeString(checksum)
		if err != nil {
			t.Fatalf("accepted non-hex checksum: %q", checksum)
		}
		if len(decoded) != 32 {
			t.Fatalf("accepted non-SHA256 digest: %q", checksum)
		}

		if asset == "" {
			t.Fatal("accepted empty asset filename")
		}
		if strings.ContainsAny(asset, `/\`) {
			t.Fatalf("accepted non-base asset filename: %q", asset)
		}

		// Independently derive the two fields from the accepted input and verify
		// that parseSignedChecksum returned exactly their documented normalized
		// forms.
		content := strings.TrimSpace(string(data))
		lines := strings.Split(content, "\n")
		if len(lines) != 1 {
			t.Fatalf("accepted input with %d lines", len(lines))
		}

		fields := strings.Fields(lines[0])
		if len(fields) != 2 {
			t.Fatalf("accepted input with %d fields: %q", len(fields), lines[0])
		}

		wantChecksum := strings.ToLower(fields[0])
		wantAsset := strings.TrimPrefix(fields[1], "*")

		if checksum != wantChecksum {
			t.Fatalf("checksum = %q, want normalized field %q", checksum, wantChecksum)
		}
		if asset != wantAsset {
			t.Fatalf("asset = %q, want normalized field %q", asset, wantAsset)
		}

		// Surrounding whitespace is explicitly ignored; it must not change the
		// parsed signed identity.
		wrapped := append([]byte(" \t\r\n"), data...)
		wrapped = append(wrapped, []byte("\r\n\t ")...)

		wrappedChecksum, wrappedAsset, wrappedErr := parseSignedChecksum(wrapped)
		if wrappedErr != nil {
			t.Fatalf("surrounding whitespace changed valid input to error: %v", wrappedErr)
		}
		if wrappedChecksum != checksum || wrappedAsset != asset {
			t.Fatalf(
				"surrounding whitespace changed result: (%q, %q) -> (%q, %q)",
				checksum,
				asset,
				wrappedChecksum,
				wrappedAsset,
			)
		}

		// Once an input is accepted, replacing a digest character with a
		// non-hexadecimal byte must make the checksum invalid.
		badDigest := []byte(checksum + "  " + fields[1])
		badDigest[0] = 'g'
		if _, _, err := parseSignedChecksum(badDigest); err == nil {
			t.Fatalf("accepted checksum after non-hex digest mutation: %q", badDigest)
		}

		// Path-qualified assets are never valid signed release asset names.
		for _, prefixedAsset := range []string{"dir/" + asset, `dir\` + asset} {
			pathInput := []byte(checksum + "  " + prefixedAsset)
			if _, _, err := parseSignedChecksum(pathInput); err == nil {
				t.Fatalf("accepted path-qualified asset %q", prefixedAsset)
			}
		}

		// A signed checksum file is deliberately a single-record format.
		canonical := checksum + "  " + fields[1]
		multiLine := []byte(canonical + "\n" + canonical)
		if _, _, err := parseSignedChecksum(multiLine); err == nil {
			t.Fatalf("accepted multiple checksum records: %q", multiLine)
		}
	})
}

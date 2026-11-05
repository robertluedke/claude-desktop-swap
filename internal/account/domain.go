package account

const domainHashPrefixLen = 32

// stripDomainPrefix drops the 32-byte SHA256 domain hash that Chromium v130+
// (cookie DB version 24) prepends to the plaintext. Cookie values are
// printable ASCII, so a non-printable decryption means the prefix is present.
func stripDomainPrefix(value []byte) []byte {
	if len(value) > domainHashPrefixLen && !isPrintableASCII(value) {
		return value[domainHashPrefixLen:]
	}
	return value
}

func isPrintableASCII(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

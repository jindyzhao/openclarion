package goodfilesystem

type EvidenceFile struct {
	Path   string
	Digest string
	Body   []byte
}

func digest(input EvidenceFile) string {
	return input.Digest
}

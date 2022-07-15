package lightning

// Invoice defines the neccessary parts for decodepayreq
type Invoice struct {
	PHash       string
	Amount      uint64
	Description string
}

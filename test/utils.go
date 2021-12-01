package test

type AssertionCounter struct {
	n int
}

func (c *AssertionCounter) Count(correct bool) {
	if !correct {
		c.n++
	}
}

func (c *AssertionCounter) HasAssertion() bool {
	return c.n > 0
}

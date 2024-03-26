package lwk

// Validator is a generic interface for validating sub configurations.
type Validator interface {
	// Validate returns an error if a particular configuration is invalid or
	// insane.
	validate() error
}

// Validate accepts a variadic list of Validators and checks that each one
// passes its Validate method. An error is returned from the first Validator
// that fails.
func Validate(validators ...Validator) error {
	for _, validator := range validators {
		if err := validator.validate(); err != nil {
			return err
		}
	}

	return nil
}

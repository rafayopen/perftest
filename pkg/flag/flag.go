package util

// StringArrayFlag should be used when flag expected an array of string arguments
type StringArrayFlag []string

func (i *StringArrayFlag) String() string {
	return "string array flag"
}

// Set is called when appending array values to StringArrayFlag
func (i *StringArrayFlag) Set(value string) error {
	*i = append(*i, value)
	return nil
}

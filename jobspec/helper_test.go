package jobspec

// These functions are copied from helper/funcs.go
// added here to avoid jobspec depending on any other package

// intToPtr returns the pointer to an int
func intToPtr(i int) *int {
	return &i
}

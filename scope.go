package thx

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"regexp"
	"runtime/debug"
)

// Scope generates an identifier specific to the current
// execution context. It uses the current stack trace to create
// a unique identifier, which is then base64 encoded and
// prefixed with the provided string. This is useful for
// generating unique ids for elements in a web application.
//
// Note: Make sure to store the result in a package
// variable, as it is expensive to compute.
func Scope(prefix string) func(string) string {
	algorithm := sha256.New()

	if _, err := algorithm.Write(staticStack()); err != nil {
		panic(err)
	}

	hashBytes := algorithm.Sum(nil)
	generatedNumber := new(big.Int).SetBytes(hashBytes).Uint64()
	finalString := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", generatedNumber)))

	if prefix != "" && prefix[len(prefix)-1] != '_' {
		prefix += "_"
	}

	prefix += finalString[:8] + "_"

	return func(val string) string {
		return prefix + val
	}
}

// staticStack returns a striped version of the debug.Stack.
// It removes the memory addresses from the stack trace, so that the
// stack is consistent across multiple calls or reboots of the app.
func staticStack() []byte {
	stack := debug.Stack()

	// match memory addresses (e.g., 0x123abc)
	regex := regexp.MustCompile(`0x[0-9a-fA-F]+`)

	lines := bytes.Split(stack, []byte{'\n'})

	for i, line := range lines {
		lines[i] = regex.ReplaceAll(line, []byte(""))
	}

	return bytes.Join(lines, []byte{'\n'})
}

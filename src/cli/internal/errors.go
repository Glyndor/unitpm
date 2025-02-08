package internal

import (
	"fmt"
	"os"
)

func Error_Fatal(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s %s\n", Color_Error("❌ ERROR:"), err.Error())
		os.Exit(1)
	}
}

func Error_Warning(message string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Color_Warning("⚠️ WARNING:"), message)
}

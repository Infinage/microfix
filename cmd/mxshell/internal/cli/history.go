package cli

import (
	"fmt"
	"os"
	"path"

	"github.com/peterh/liner"
)

func historyPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("Failed to resolve UserHomeDir and CurrentWorkingDirectory")
		}
	}
	return path.Join(homeDir, ".mxhistory"), nil
}

func loadHistory(line *liner.State) {
	historyFp, err := historyPath()
	if err != nil {
		return
	}
	if f, err := os.Open(historyFp); err == nil {
		line.ReadHistory(f)
		f.Close()
	}
}

func writeHistory(line *liner.State) {
	historyFp, err := historyPath()
	if err != nil {
		return
	}
	if f, err := os.Create(historyFp); err == nil {
		line.WriteHistory(f)
		f.Close()
	}
}

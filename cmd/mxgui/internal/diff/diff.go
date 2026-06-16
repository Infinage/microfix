package diff

import (
	"slices"

	"github.com/infinage/microfix/pkg/message"
)

type DiffStatus int

const (
	DiffEqual    DiffStatus = iota // Tag + Value match
	DiffModified                   // Tag match
	DiffAdded                      // Found in Target only
	DiffRemoved                    // Found in Source only
)

type DiffRow struct {
	Tag    uint16
	Name   string
	Source string
	Target string
	Status DiffStatus
}

func Compare(source, target message.Message) []DiffRow {
	// Init longest common subsequence (LCS) grid
	n1, n2 := len(source), len(target)
	dp := make([][]int, n1+1)
	for i := range dp {
		dp[i] = make([]int, n2+1)
	}

	// Weighted LCS
	for i := range n1 {
		for j := range n2 {
			dp[i+1][j+1] = max(dp[i][j+1], dp[i+1][j])
			if source[i].Tag == target[j].Tag {
				weight := 1 // Tag match
				if source[i].Value == target[j].Value {
					weight = 2 // Exact match
				}
				dp[i+1][j+1] = max(dp[i+1][j+1], dp[i][j]+weight)
			}
		}
	}

	// Backtrack to find the LCS
	var result []DiffRow
	i, j := n1, n2
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && source[i-1].Tag == target[j-1].Tag {
			// Choose between DiffEqual and DiffModified
			weight := 1
			status := DiffModified
			if source[i-1].Value == target[j-1].Value {
				weight = 2
				status = DiffEqual
			}

			// Prefer diagonal only when it is responsible for current score
			// Otherwise choose between add or remove
			if dp[i][j] == dp[i-1][j-1]+weight {
				result = append(result, DiffRow{
					Tag:    source[i-1].Tag,
					Source: source[i-1].Value,
					Target: target[j-1].Value,
					Status: status,
				})

				i--
				j--
				continue
			}
		}

		if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			// Item is in target but missing in source
			result = append(result, DiffRow{
				Tag:    target[j-1].Tag,
				Source: "",
				Target: target[j-1].Value,
				Status: DiffAdded,
			})

			j--
		} else {
			// Item is in source but missing in target
			result = append(result, DiffRow{
				Tag:    source[i-1].Tag,
				Source: source[i-1].Value,
				Target: "",
				Status: DiffRemoved,
			})

			i--
		}
	}

	slices.Reverse(result)
	return result
}

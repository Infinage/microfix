package migrate

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/infinage/microfix/pkg/message"
)

type minifixConfig struct {
	Count int `xml:"transConf>Software_MiniFIX_Transaction>count"`
	Items []struct {
		Name string `xml:"first"`
		Raw  string `xml:"second"`
	} `xml:"transConf>Software_MiniFIX_Transaction>item"`
}

// parseMiniFIXTransaction parses format as "0 0 1 0 5 +35=l" into microfix message struct
func parseMiniFIXTransaction(raw string) (message.Message, error) {
	r := strings.NewReader(raw)
	var discard, count int
	if _, err := fmt.Fscanf(r, "%d %d %d %d", &discard, &discard, &count, &discard); err != nil {
		return nil, fmt.Errorf("expected: `%%d %%d %%d %%d ...`: %w", err)
	}

	// Buffer to hold the message
	var msg message.Message

	// Temporaries to process loop
	var size int
	var tag uint64
	var err error

	for i := range count {
		// Parse current field
		if _, err := fmt.Fscan(r, &size); err != nil {
			return nil, fmt.Errorf("failed parsing field #%d: %w", i, err)
		}

		// Skip if empty string, ideally we should insert an empty field into
		// result to ensure we don't break ordering but that wouldn't be a valid FIX
		if size == 0 {
			continue
		}

		// Discard whitespace
		if b, err := r.ReadByte(); err != nil || b != ' ' {
			return nil, fmt.Errorf("expected space after size, got %q (err: %v)", b, err)
		}

		// Read '+35=D'
		buf := make([]byte, size)
		if n, err := r.Read(buf); n != size || err != nil {
			return nil, fmt.Errorf("expected %d bytes, failed: %w", size, err)
		}

		// Ensure field itself is valid
		// Note: we include both '+' and '-' fields, ideally we should ignore '-'
		// but we are favouring retaining as much tag as possible from tranConfig.
		if marker := buf[0]; marker != '+' && marker != '-' {
			return nil, fmt.Errorf("unknown marker %c", marker)
		}
		tagStr, value, ok := strings.Cut(string(buf[1:]), "=")
		if !ok {
			return nil, fmt.Errorf("field missing '=' seperator (#%d)", i)
		}
		if tag, err = strconv.ParseUint(tagStr, 10, 16); err != nil {
			return nil, fmt.Errorf("invalid tag %s parsing field #%d", buf, i)
		}

		msg = append(msg, message.Field{Tag: uint16(tag), Value: value})
	}

	return msg, nil
}

// parseMiniFIXTransactions converts loaded MiniFIX transactions into MicroFIX aliases.
// It returns the successfully parsed aliases and the names of transactions that could not be parsed.
func parseMiniFIXTransactions(mcfg minifixConfig) (alias map[string]message.Message, failed []string, err error) {
	if actualCount := len(mcfg.Items); mcfg.Count != actualCount {
		return nil, nil, fmt.Errorf("expected %d items, found %d", mcfg.Count, actualCount)
	}

	data := make(map[string]message.Message)
	for _, item := range mcfg.Items {
		if parsed, err := parseMiniFIXTransaction(item.Raw); err == nil {
			data[item.Name] = parsed
		} else {
			failed = append(failed, item.Name)
		}
	}

	return data, failed, nil
}

// ExtractAliasFromMiniFIX reads fields from 'Config>transConf>Software_MiniFIX_Transaction'
// and returns a MicroFIX-compatible alias map along with the names of transactions that could not be parsed.
func ExtractAliasFromMiniFIX(r io.Reader) (alias map[string]message.Message, failed []string, err error) {
	var data minifixConfig
	dec := xml.NewDecoder(r)
	if err := dec.Decode(&data); err != nil {
		return nil, nil, fmt.Errorf("minifix xml parse failed: %w", err)
	}
	return parseMiniFIXTransactions(data)
}

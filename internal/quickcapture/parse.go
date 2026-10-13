// Package quickcapture parses one-line quick-capture strings like
// "buy milk @tomorrow 9:00 #groceries".
package quickcapture

import (
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/vladgrskkh/psst/internal/dateparser"
	"github.com/vladgrskkh/psst/pkg/option"
)

var ErrEmptyTitle = errors.New("todo title must not be empty")

var (
	reTime    = regexp.MustCompile(`^\d{1,2}:\d{2}$`)
	reTimeAMP = regexp.MustCompile(`^\d{1,2}\s*(am|pm)$`)
)

type Result struct {
	Title    string
	Deadline option.Option[time.Time]
	List     option.Option[string]
}

func isTimeToken(s string) bool {
	return reTime.MatchString(s) || reTimeAMP.MatchString(s)
}

// Parse extracts title, deadline hint and list from a quick-capture string.
func Parse(s string, now time.Time, defaultTime string) (*Result, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ErrEmptyTitle
	}
	tokens := strings.Fields(s)
	res := &Result{}
	titleParts := make([]string, 0, len(tokens))

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if strings.HasPrefix(tok, "#") {
			if list, ok := listFromTokens(tok, tokens[i+1:]); ok {
				res.List = option.Some(list)
			}
			i = len(tokens)
			continue
		}
		if strings.HasPrefix(tok, "@") {
			deadline, next, err := parseDeadline(tokens, i, now, defaultTime)
			if err != nil {
				return nil, err
			}
			res.Deadline = deadline
			i = next
			continue
		}
		titleParts = append(titleParts, tok)
	}

	res.Title = strings.TrimSpace(strings.Join(titleParts, " "))
	if res.Title == "" {
		return nil, ErrEmptyTitle
	}
	return res, nil
}

func listFromTokens(tok string, rest []string) (string, bool) {
	list := strings.TrimSpace(strings.TrimPrefix(tok, "#"))
	if len(rest) > 0 {
		list = strings.TrimSpace(list + " " + strings.Join(rest, " "))
	}
	return list, list != ""
}

func parseDeadline(tokens []string, i int, now time.Time, defaultTime string) (option.Option[time.Time], int, error) {
	hintParts := []string{strings.TrimPrefix(tokens[i], "@")}
	for i+1 < len(tokens) && (isTimeToken(tokens[i+1]) || tokens[i+1] == "at") {
		i++
		hintParts = append(hintParts, tokens[i])
	}
	hint := strings.Join(hintParts, " ")
	if hint == "" {
		return option.None[time.Time](), i, nil
	}
	deadline, err := dateparser.Parse(hint, now, defaultTime)
	if err != nil {
		return option.None[time.Time](), i, err
	}
	return option.Some(deadline), i, nil
}

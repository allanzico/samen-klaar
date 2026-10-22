// Package matching verifies published service scope before ranking availability.
package matching

import (
	"context"
	"errors"
	"github.com/allanzico/samen-klaar/internal/workflow"
	"strings"
)

type Service struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Materials   string   `json:"materials"`
	Exclusions  []string `json:"exclusions"`
}
type Fit struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
}
type Selection struct {
	Fits     []Fit  `json:"fits"`
	Question string `json:"question"`
}
type Directory interface {
	ServiceCatalog(context.Context, string, string, string, ...workflow.Location) ([]Service, error)
	ServiceMatches(context.Context, string, string, Service, ...workflow.Location) ([]workflow.Match, error)
}
type Selector interface {
	SelectServices(context.Context, string, []Service) (Selection, error)
}

func Find(ctx context.Context, directory Directory, selector Selector, category, description, exclude string, locations ...workflow.Location) ([]workflow.Match, string, error) {
	if category == "Unclear" {
		return nil, "Where is the problem, and what happens when you notice it?", nil
	}
	services, err := directory.ServiceCatalog(ctx, category, description, exclude, locations...)
	if err != nil {
		return nil, "", err
	}
	if len(services) == 0 {
		if len(locations) > 0 && strings.TrimSpace(locations[0].Label) != "" {
			return nil, "No approved professional has published a matching service for this area yet. You can save the request to the area waitlist.", nil
		}
		return nil, "I couldn’t find a published service for this request. What part needs attention, and what is happening?", nil
	}
	selection, err := selector.SelectServices(ctx, description, services)
	if err != nil {
		return nil, "", err
	}
	matches := []workflow.Match{}
	seen := map[int]bool{}
	if len(selection.Fits) > 4 {
		return nil, "", errors.New("too many service selections")
	}
	for _, fit := range selection.Fits {
		reason := displayFitReason(fit.Reason)
		if fit.Index < 0 || fit.Index >= len(services) || seen[fit.Index] || len(reason) < 8 {
			return nil, "", errors.New("invalid service selection")
		}
		seen[fit.Index] = true
		candidates, err := directory.ServiceMatches(ctx, category, exclude, services[fit.Index], locations...)
		if err != nil {
			return nil, "", err
		}
		for _, candidate := range candidates {
			candidate.FitReason = reason
			matches = append(matches, candidate)
		}
	}
	if len(locations) > 0 {
		workflow.OrderMatches(matches, &locations[0])
	} else {
		workflow.OrderMatches(matches, nil)
	}
	if len(matches) > 3 {
		matches = matches[:3]
	}
	if len(matches) == 0 {
		if len(seen) > 0 && len(locations) > 0 && strings.TrimSpace(locations[0].Label) != "" {
			return matches, "No approved professional with a published service covers this area yet. You can save the request to the area waitlist.", nil
		}
		question := selection.Question
		if len(question) < 8 || len(question) > 240 {
			question = "What part needs attention, and what is happening? I need a little more detail to match the published service."
		}
		return matches, question, nil
	}
	return matches, "", nil
}

// Explanation length is a display concern, not service eligibility. Providers
// can return a valid selection with longer copy; keep it readable without
// discarding the matching professionals or splitting a Unicode character.
func displayFitReason(reason string) string {
	reason = strings.TrimSpace(reason)
	runes := []rune(reason)
	if len(runes) <= 240 {
		return reason
	}
	return strings.TrimSpace(string(runes[:239])) + "…"
}

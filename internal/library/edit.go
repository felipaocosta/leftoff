package library

import (
	"context"
	"fmt"
	"strings"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// Patch lists the user-editable fields of a series (RF-05, RF-07). Nil
// fields are left unchanged.
type Patch struct {
	Title     *string            `json:"title,omitempty"`
	AltTitles *[]string          `json:"alt_titles,omitempty"`
	Type      *domain.SeriesType `json:"type,omitempty"`
	Tags      *[]string          `json:"tags,omitempty"`
	Notes     *string            `json:"notes,omitempty"`
	Category  *domain.Category   `json:"category,omitempty"`
}

func (p Patch) validate() error {
	if p.Title != nil && strings.TrimSpace(*p.Title) == "" {
		return fmt.Errorf("title must not be empty: %w", domain.ErrInvalidInput)
	}
	if p.Type != nil && !p.Type.Valid() {
		return fmt.Errorf("type %q: %w", *p.Type, domain.ErrInvalidInput)
	}
	if p.Category != nil && !p.Category.Valid() {
		return fmt.Errorf("category %q: %w", *p.Category, domain.ErrInvalidInput)
	}
	return nil
}

// Edit applies a patch to a series. Invalid patches change nothing.
func (s *Service) Edit(ctx context.Context, id string, p Patch) (domain.Series, error) {
	if err := p.validate(); err != nil {
		return domain.Series{}, err
	}
	return s.updateSeries(ctx, id, func(sr *domain.Series) error {
		if p.Title != nil {
			sr.Title = strings.TrimSpace(*p.Title)
		}
		if p.AltTitles != nil {
			sr.AltTitles = cleanList(*p.AltTitles)
		}
		if p.Type != nil {
			sr.Type = *p.Type
		}
		if p.Tags != nil {
			sr.Tags = cleanList(*p.Tags)
		}
		if p.Notes != nil {
			sr.Notes = *p.Notes
		}
		if p.Category != nil {
			sr.Category = *p.Category
		}
		return nil
	})
}

// cleanList trims items and drops empty ones and repeats, keeping order.
func cleanList(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

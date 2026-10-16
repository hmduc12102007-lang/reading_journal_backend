package handlers

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"reading_journal_backend/internal/models"
)

func validateBookFields(book models.Book) string {
	if strings.TrimSpace(book.Title) == "" || utf8.RuneCountInString(book.Title) > 255 {
		return "title must contain 1 to 255 characters"
	}
	if strings.TrimSpace(book.Author) == "" || utf8.RuneCountInString(book.Author) > 255 {
		return "author must contain 1 to 255 characters"
	}
	for _, field := range []struct {
		name  string
		value *string
		max   int
	}{
		{"publisher", book.Publisher, 255}, {"isbn", book.ISBN, 32}, {"cover_url", book.CoverURL, 2048},
	} {
		if field.value != nil && utf8.RuneCountInString(*field.value) > field.max {
			return field.name + " is too long"
		}
	}
	if book.CoverURL != nil {
		value := *book.CoverURL
		// Local cover URLs can only point to this book's protected endpoint.
		if value == coverURL(book.BookID) && book.BookID != 0 && book.CoverFile != nil {
			return ""
		}
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil {
			return "cover_url must be an absolute HTTP or HTTPS URL; use the upload endpoint for local images"
		}
	}
	return ""
}

package arrapi

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ReadarrClient provides API access to Readarr
type ReadarrClient struct {
	*Client
}

// NewReadarrClient creates a new Readarr API client
func NewReadarrClient(cfg ClientConfig) *ReadarrClient {
	return &ReadarrClient{
		Client: NewClient(cfg),
	}
}

// Author represents a Readarr author
type Author struct {
	ID                int       `json:"id"`
	ForeignAuthorID   string    `json:"foreignAuthorId"`
	AuthorName        string    `json:"authorName"`
	CleanName         string    `json:"cleanName"`
	Monitored         bool      `json:"monitored"`
	Status            string    `json:"status"`
	Overview          string    `json:"overview"`
	Path              string    `json:"path"`
	QualityProfileID  int       `json:"qualityProfileId"`
	MetadataProfileID int       `json:"metadataProfileId"`
	Added             time.Time `json:"added"`
	Statistics        *struct {
		BookCount      int     `json:"bookCount"`
		BookFileCount  int     `json:"bookFileCount"`
		TotalBookCount int     `json:"totalBookCount"`
		SizeOnDisk     int64   `json:"sizeOnDisk"`
		PercentOfBooks float64 `json:"percentOfBooks"`
	} `json:"statistics,omitempty"`
}

// Book represents a Readarr book
type Book struct {
	ID            int       `json:"id"`
	ForeignBookID string    `json:"foreignBookId"`
	Title         string    `json:"title"`
	CleanTitle    string    `json:"cleanTitle"`
	Monitored     bool      `json:"monitored"`
	AuthorID      int       `json:"authorId"`
	ReleaseDate   time.Time `json:"releaseDate"`
	Overview      string    `json:"overview"`
	Genres        []string  `json:"genres"`
	PageCount     int       `json:"pageCount"`
	Statistics    *struct {
		BookFileCount  int     `json:"bookFileCount"`
		SizeOnDisk     int64   `json:"sizeOnDisk"`
		PercentOfBooks float64 `json:"percentOfBooks"`
	} `json:"statistics,omitempty"`
}

// GetAuthor retrieves an author by ID
func (c *ReadarrClient) GetAuthor(ctx context.Context, id int) (*Author, error) {
	endpoint := fmt.Sprintf("author/%d", id)

	var author Author
	if err := c.get(ctx, endpoint, &author); err != nil {
		return nil, fmt.Errorf("failed to get author %d: %w", id, err)
	}

	return &author, nil
}

// GetBook retrieves a book by ID
func (c *ReadarrClient) GetBook(ctx context.Context, id int) (*Book, error) {
	endpoint := fmt.Sprintf("book/%d", id)

	var book Book
	if err := c.get(ctx, endpoint, &book); err != nil {
		return nil, fmt.Errorf("failed to get book %d: %w", id, err)
	}

	return &book, nil
}

// SearchBook triggers a search for a book by ID
func (c *ReadarrClient) SearchBook(ctx context.Context, bookID int) error {
	body := strings.NewReader(fmt.Sprintf(`{"name":"BookSearch","bookIds":[%d]}`, bookID))

	if err := c.post(ctx, "command", body); err != nil {
		return fmt.Errorf("failed to search book %d: %w", bookID, err)
	}

	return nil
}

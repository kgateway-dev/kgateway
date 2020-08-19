package main

import (
	"context"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/pkg/errors"
)

func NewServer() BookstoreServer {
	return &bookstoreServer{
		shelves: make(map[int64]*Shelf),
		books:   make(map[int64][]*Book),
	}
}

type bookstoreServer struct {
	shelves map[int64]*Shelf
	books   map[int64][]*Book
	m       sync.RWMutex
}

// Returns a list of all shelves in the bookstore.
func (s *bookstoreServer) ListShelves(context.Context, *empty.Empty) (*ListShelvesResponse, error) {
	var shelves []*Shelf
	for _, shelf := range s.shelves {
		shelves = append(shelves, shelf)
	}
	return &ListShelvesResponse{
		Shelves: shelves,
	}, nil
}

// Creates a new shelf in the bookstore.
func (s *bookstoreServer) CreateShelf(ctx context.Context, r *CreateShelfRequest) (*Shelf, error) {
	s.m.Lock()
	defer s.m.Unlock()
	if r == nil {
		return nil, errors.New("request cannot be nil")
	}
	if r.Shelf == nil {
		return nil, errors.New("shelf cannot be nil")
	}
	s.shelves[r.Shelf.Id] = r.Shelf
	return r.Shelf, nil
}

// Creates multiple shelves with one streaming call
func (s *bookstoreServer) BulkCreateShelf(stream Bookstore_BulkCreateShelfServer) error {
	for {
		select {
		case <-stream.Context().Done():
			return nil
		default:
		}
		req, err := stream.Recv()
		if err != nil {
			return errors.Wrap(err, "receiving next create req")
		}
		if _, err := s.CreateShelf(stream.Context(), req); err != nil {
			return errors.Wrap(err, "creating shelf")
		}
	}
}

// Returns a specific bookstore shelf.
func (s *bookstoreServer) GetShelf(ctx context.Context, r *GetShelfRequest) (*Shelf, error) {
	s.m.RLock()
	defer s.m.RUnlock()
	if shelf, ok := s.shelves[r.Shelf]; ok {
		return shelf, nil
	}
	return nil, errors.Errorf("shelf %v not found", r.Shelf)
}

// Deletes a shelf, including all books that are stored on the shelf.
func (s *bookstoreServer) DeleteShelf(ctx context.Context, r *DeleteShelfRequest) (*empty.Empty, error) {
	s.m.Lock()
	defer s.m.RUnlock()
	delete(s.shelves, r.Shelf)
	return nil, nil
}

// Returns a list of books on a shelf.
func (s *bookstoreServer) ListBooks(req *ListBooksRequest, stream Bookstore_ListBooksServer) error {
	shelf, err := s.GetShelf(stream.Context(), &GetShelfRequest{Shelf: req.Shelf})
	if err != nil {
		return err
	}
	s.m.RLock()
	defer s.m.RUnlock()
	for _, book := range s.books[shelf.Id] {
		if err := stream.Send(book); err != nil {
			return errors.Wrap(err, "stream error")
		}
	}
	return nil
}

// Creates a new book.
func (s *bookstoreServer) CreateBook(ctx context.Context, req *CreateBookRequest) (*Book, error) {
	shelf, err := s.GetShelf(ctx, &GetShelfRequest{Shelf: req.Shelf})
	if err != nil {
		return nil, err
	}

	s.m.Lock()
	defer s.m.Unlock()
	s.books[shelf.Id] = append(s.books[shelf.Id], req.Book)
	return req.Book, nil
}

// Returns a specific book.
func (s *bookstoreServer) GetBook(ctx context.Context, req *GetBookRequest) (*Book, error) {
	shelf, err := s.GetShelf(ctx, &GetShelfRequest{Shelf: req.Shelf})
	if err != nil {
		return nil, err
	}
	s.m.RLock()
	defer s.m.RUnlock()
	for _, book := range s.books[shelf.Id] {
		if book.Id == req.Book {
			return book, nil
		}
	}
	return nil, errors.Errorf("book %v not found", req.Book)
}

// Deletes a book from a shelf.
func (s *bookstoreServer) DeleteBook(ctx context.Context, req *DeleteBookRequest) (*empty.Empty, error) {
	s.m.Lock()
	defer s.m.Unlock()
	if books, ok := s.books[req.Shelf]; ok {
		for i, book := range books {
			if book.Id == req.Book {
				s.books[req.Shelf] = append(books[:i], books[i+1:]...)
				return &empty.Empty{}, nil
			}
		}
	}
	return nil, errors.Errorf("book %v not found for shelf %v", req.Book, req.Shelf)
}

func (s *bookstoreServer) UpdateBook(ctx context.Context, req *UpdateBookRequest) (*Book, error) {
	s.m.Lock()
	defer s.m.Unlock()
	if books, ok := s.books[req.Shelf]; ok {
		for i, book := range books {
			if book.Id == req.Book.Id {
				s.books[req.Shelf] = append(append(books[:i], req.Book), books[i+1:]...)
				return req.Book, nil
			}
		}
	}
	return nil, errors.Errorf("book %v not found for shelf %v", req.Book, req.Shelf)
}

func (s *bookstoreServer) BookstoreOptions(context.Context, *GetShelfRequest) (*empty.Empty, error) {
	return &empty.Empty{}, nil
}

// Returns a specific author.
func (s *bookstoreServer) GetAuthor(ctx context.Context, req *GetAuthorRequest) (*Author, error) {
	return nil, errors.New("unsupported operation")
}

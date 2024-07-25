package iosnapshot

import "fmt"

// SnapshotResponseData is the data that is returned by Getter methods on the History object
// It allows us to encapsulate data and errors together, so that if an issue occurs during the request,
// we can get access to all the relevant information
type SnapshotResponseData struct {
	Status SnapshotResponseStatus
	Data   string
	Error  error
}

// SnapshotResponseStatus identifies the status of the SnapshotResponse
type SnapshotResponseStatus int

const (
	// Complete signifies that the entire requested snapshot is returned
	Complete SnapshotResponseStatus = iota

	// Error signifies that the requested snapshot could not be returned, due to an error
	Error
)

func (s SnapshotResponseStatus) String() string {
	switch s {
	case Complete:
		return "Complete"
	case Error:
		return "Error"
	default:
		return fmt.Sprintf("%d", int(s))
	}
}

func completeSnapshotResponse(bytes []byte) SnapshotResponseData {
	return SnapshotResponseData{
		Status: Complete,
		Data:   string(bytes),
		Error:  nil,
	}
}

func errorSnapshotResponse(err error) SnapshotResponseData {
	return SnapshotResponseData{
		Status: Error,
		Data:   "",
		Error:  err,
	}
}

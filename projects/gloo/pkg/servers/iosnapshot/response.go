package iosnapshot

import "encoding/json"

// SnapshotResponseData is the data that is returned by Getter methods on the History object
// It allows us to encapsulate data and errors together, so that if an issue occurs during the request,
// we can get access to all the relevant information
type SnapshotResponseData struct {
	Data  string `json:"data"`
	Error error  `json:"error"`
}

func (r SnapshotResponseData) MarshalJSON() ([]byte, error) {
	// See: https://github.com/golang/go/issues/5161#issuecomment-1750037535
	var errorMsg string
	if r.Error != nil {
		errorMsg = r.Error.Error()
	}
	anon := struct {
		Data  string `json:"data"`
		Error string `json:"error"`
	}{
		Data:  r.Data,
		Error: errorMsg,
	}
	return json.Marshal(anon)
}

func (r SnapshotResponseData) MarshalJSONString() string {
	bytes, err := r.MarshalJSON()
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}

func completeSnapshotResponse(bytes []byte) SnapshotResponseData {
	return SnapshotResponseData{
		Data:  string(bytes),
		Error: nil,
	}
}

func errorSnapshotResponse(err error) SnapshotResponseData {
	return SnapshotResponseData{
		Data:  "",
		Error: err,
	}
}

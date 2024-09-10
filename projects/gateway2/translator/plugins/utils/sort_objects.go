package utils

import (
	"fmt"
	"reflect"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NOTE: this sort utility is adapted from GME policy sorting
// ref: pkg/translator/utils/sort_creation_timestamp.go

// SortByCreationTime accepts a slice of client.Object instances and sorts it by creation timestamp in ascending order.
// It panics if the argument isn't a slice, or if it is a slice of a type that does not implement client.Object.
func SortByCreationTime(objs interface{}) {
	// Validate the argument
	if err := validate(objs); err != nil {
		panic(err)
	}

	// If we got past validation, the argument is either an empty slice or a slice of client.Object.
	// In the former case the comparison function will not be invoked, in the latter we can safely use getCreationTime.
	sort.SliceStable(objs, func(i, j int) bool {
		iTime := getCreationTime(objs, i)
		jTime := getCreationTime(objs, j)

		if iTime.Equal(&jTime) {
			iWorkload := getObjectName(objs, i)
			jWorkload := getObjectName(objs, j)
			return iWorkload < jWorkload
		}

		return iTime.Before(&jTime)
	})
}

func validate(objs interface{}) error {
	s := reflect.ValueOf(objs)

	if s.Kind() != reflect.Slice {
		return fmt.Errorf("argument must be a slice")
	}

	if s.IsNil() {
		return nil
	}

	for i := range s.Len() {
		el := s.Index(i).Interface()
		if _, ok := el.(client.Object); !ok {
			return fmt.Errorf("input slice contains element of unexpected type %T; all elements must implement client.Object", el)
		}
	}

	return nil
}

func getCreationTime(objs interface{}, index int) metav1.Time {
	return reflect.ValueOf(objs).Index(index).Interface().(client.Object).GetCreationTimestamp()
}

// returns string in format 'namespace/name'
func getObjectName(objs interface{}, index int) string {
	obj := reflect.ValueOf(objs).Index(index).Interface().(client.Object)
	return client.ObjectKeyFromObject(obj).String()
}

package stringutils

// Only deletes the first instance of value!
// Takes a slice and a value and if that value is found, a new slice is returned
// with the value removed. If the value is not found, the original slice is returned.
func DeleteOneByValue(slice []string, value string) []string {
	for i, v := range slice {
		if v == value {
			return DeleteAtIndex(slice, i)
		}
	}
	return slice
}

// Adapted from https://www.geeksforgeeks.org/delete-elements-in-a-slice-in-golang/
// Function that takes two parameters
// a slice which has to be operated on
// the index of the element to be deleted from the slice
// return value as a slice of integers
func DeleteAtIndex(slice []string, index int) []string {

	// Append function used to append elements to a slice
	// first parameter as the slice to which the elements
	// are to be added/appended second parameter is the
	// element(s) to be appended into the slice
	// return value as a slice
	return append(slice[:index], slice[index+1:]...)
}

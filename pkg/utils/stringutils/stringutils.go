package stringutils

import slices "golang.org/x/exp/slices"

// Only deletes the first instance of value!
// Takes a slice and a value and if that value is found, uses Delete from the exp.slices package to remove it.
// Otherwise returns the original slice.
func DeleteOneByValue(slice []string, value string) []string {
	index := slices.Index(slice, value)
	if index == -1 {
		return slice
	}
	return slices.Delete(slice, index, index+1)

}

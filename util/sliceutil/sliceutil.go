package sliceutil

import (
	"bytes"
	"reflect"
)

/* Based on assert.includeElements() of https://github.com/stretchr/testify
   Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell and contributors.
   MIT License
*/

// containsElement try loop over the list check if the list includes the element.
// return (false, false) if impossible.
// return (true, false) if element was not found.
// return (true, true) if element was found.
func Contains(slice interface{}, element interface{}) (found bool) { //nolint:nonamedreturns
	listValue := reflect.ValueOf(slice)
	defer func() {
		if e := recover(); e != nil {
			found = false
		}
	}()

	for i := 0; i < listValue.Len(); i++ {
		if objectsAreEqual(listValue.Index(i).Interface(), element) {
			return true
		}
	}
	return false
}

// objectsAreEqual determines if two objects are considered equal.
//
// This function does no assertion of any kind.
func objectsAreEqual(expected, actual interface{}) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}

	exp, ok := expected.([]byte)
	if !ok {
		return reflect.DeepEqual(expected, actual)
	}

	act, ok := actual.([]byte)
	if !ok {
		return false
	}
	if exp == nil || act == nil {
		return exp == nil && act == nil
	}
	return bytes.Equal(exp, act)
}

func Equal[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func RemoveDuplicates[T comparable](slice []T) []T {
	res := make([]T, 0, len(slice))
	keys := make(map[T]struct{}, len(slice))
	for _, element := range slice {
		if _, exists := keys[element]; exists {
			continue
		}
		keys[element] = struct{}{}
		res = append(res, element)
	}
	return res
}

package utils

import (
	"github.com/microsoft/TypeScript/tsc/shim/checker"
)

func MatchesTypeOrBaseType(typeChecker *checker.Checker, t *checker.Type, predicate func(*checker.Type) bool) bool {
	if predicate(t) {
		return true
	}

	if !IsObjectType(t) {
		return false
	}

	target := t
	if t.ObjectFlags()&checker.ObjectFlagsReference != 0 {
		target = t.Target()
	}

	if target.ObjectFlags()&checker.ObjectFlagsClassOrInterface != 0 {
		for _, baseType := range typeChecker.GetBaseTypes(target) {
			if MatchesTypeOrBaseType(typeChecker, baseType, predicate) {
				return true
			}
		}
	}

	return false
}

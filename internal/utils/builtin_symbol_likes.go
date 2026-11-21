package utils

import (
	"github.com/microsoft/TypeScript/tsc/shim/ast"
	"github.com/microsoft/TypeScript/tsc/shim/checker"
	"github.com/microsoft/TypeScript/tsc/shim/compiler"
	"github.com/microsoft/TypeScript/tsc/shim/tsoptions"
	"github.com/microsoft/TypeScript/tsc/shim/tspath"
)

func ComparePaths(a string, b string, program *compiler.Program) int {
	return program.Host().FS().CaseSensitivity().CompareFilePaths(tspath.ToRootedFilePath(a, program.GetCurrentDirectory()), tspath.ToRootedFilePath(b, program.GetCurrentDirectory()))
}

func IsSourceFileDefaultLibrary(program *compiler.Program, file *ast.SourceFile) bool {
	if !file.IsDeclarationFile {
		return false
	}

	if program.IsSourceFileDefaultLibrary(file.PathKey()) {
		return true
	}

	options := program.Options()

	if options.NoLib.IsTrue() {
		return false
	}

	// copied from program.go
	var libs []string
	if options.Lib == nil {
		name := tsoptions.GetDefaultLibFileName(options)
		libs = append(libs, tspath.CombinePaths(program.Host().DefaultLibraryPath().AsString(), name))
	} else {
		for _, lib := range options.Lib {
			name, ok := tsoptions.GetLibFileName(lib)
			if ok {
				libs = append(libs, tspath.CombinePaths(program.Host().DefaultLibraryPath().AsString(), name))
			}
			// !!! error on unknown name
		}
	}

	return Some(libs, func(lib string) bool {
		return ComparePaths(file.FileName().AsString(), lib, program) == 0
	})
}

func IsSymbolFromDefaultLibrary(
	program *compiler.Program,
	symbol *ast.Symbol,
) bool {
	if symbol == nil {
		return false
	}

	for _, declaration := range symbol.Declarations() {
		sourceFile := ast.GetSourceFileOfNode(declaration)
		if IsSourceFileDefaultLibrary(program, sourceFile) {
			return true
		}
	}

	return false
}

/**
 * @example
 * ```ts
 * class DerivedClass extends Promise<number> {}
 * DerivedClass.reject
 * // ^ PromiseLike
 * ```
 */
func IsPromiseLike(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type) bool {
	return IsBuiltinSymbolLike(program, typeChecker, t, "Promise")
}

/**
 * @example
 * ```ts
 * const value = Promise
 * value.reject
 * // ^ PromiseConstructorLike
 * ```
 */
func IsPromiseConstructorLike(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type,
) bool {
	return IsBuiltinSymbolLike(program, typeChecker, t, "PromiseConstructor")
}

/**
 * @example
 * ```ts
 * class Foo extends Error {}
 * new Foo()
 * //   ^ ErrorLike
 * ```
 */
func IsErrorLike(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type) bool {
	return IsBuiltinSymbolLike(program, typeChecker, t, "Error")
}

/**
 * @example
 * ```ts
 * type T = Readonly<Error>
 * //   ^ ReadonlyErrorLike
 * ```
 */
func IsReadonlyErrorLike(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type,
) bool {
	return IsReadonlyTypeLike(program, typeChecker, t, func(subtype *checker.Type) bool {
		subtype.Alias().TypeArguments()
		typeArgument := subtype.Alias().TypeArguments()[0]

		return IsErrorLike(program, typeChecker, typeArgument) || IsReadonlyErrorLike(program, typeChecker, typeArgument)
	})
}

/**
 * @example
 * ```ts
 * type T = Readonly<{ foo: 'bar' }>
 * //   ^ ReadonlyTypeLike
 * ```
 */
func IsReadonlyTypeLike(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type,
	predicate func(subType *checker.Type) bool,
) bool {
	return IsBuiltinTypeAliasLike(program, typeChecker, t, func(subtype *checker.Type) bool {
		return subtype.Alias().Symbol().Name() == "Readonly" && predicate(subtype)
	})
}

type builtinPredicateMatches uint8

const (
	builtinPredicateMatches_Unknown builtinPredicateMatches = iota
	builtinPredicateMatches_False
	builtinPredicateMatches_True
)

func IsBuiltinTypeAliasLike(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type,
	predicate func(subType *checker.Type) bool,
) bool {
	return IsBuiltinSymbolLikeRecurser(program, typeChecker, t, func(subtype *checker.Type) builtinPredicateMatches {
		aliasSymbol := subtype.Alias()
		if aliasSymbol == nil || len(aliasSymbol.TypeArguments()) == 0 {
			return builtinPredicateMatches_False
		}

		if IsSymbolFromDefaultLibrary(program, aliasSymbol.Symbol()) && predicate(subtype) {
			return builtinPredicateMatches_True
		}

		return builtinPredicateMatches_Unknown
	})
}

func IsBuiltinSymbolLike(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type,
	symbolNames ...string,
) bool {
	return IsBuiltinSymbolLikeRecurser(program, typeChecker, t, func(subType *checker.Type) builtinPredicateMatches {
		symbol := subType.Symbol()
		if symbol == nil {
			return builtinPredicateMatches_False
		}

		actualSymbolName := symbol.Name()

		if Some(symbolNames, func(name string) bool { return actualSymbolName == name }) && IsSymbolFromDefaultLibrary(program, symbol) {
			return builtinPredicateMatches_True
		}

		return builtinPredicateMatches_Unknown
	})
}

func IsAnyBuiltinSymbolLike(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type,
) bool {
	return IsBuiltinSymbolLikeRecurser(program, typeChecker, t, func(subType *checker.Type) builtinPredicateMatches {
		symbol := subType.Symbol()
		if symbol == nil {
			return builtinPredicateMatches_False
		}

		if IsSymbolFromDefaultLibrary(program, symbol) {
			return builtinPredicateMatches_True
		}

		return builtinPredicateMatches_Unknown
	})
}

func IsBuiltinSymbolLikeRecurser(
	program *compiler.Program,
	typeChecker *checker.Checker,
	t *checker.Type,
	predicate func(subType *checker.Type) builtinPredicateMatches,
) bool {
	if IsIntersectionType(t) {
		return Some(IntersectionTypeParts(t), func(t *checker.Type) bool {
			return IsBuiltinSymbolLikeRecurser(program, typeChecker, t, predicate)
		})
	}
	if IsUnionType(t) {
		return Every(UnionTypeParts(t), func(t *checker.Type) bool {
			return IsBuiltinSymbolLikeRecurser(program, typeChecker, t, predicate)
		})
	}
	if IsTypeParameter(t) {
		constraint := typeChecker.GetBaseConstraintOfType(t)

		if constraint != nil {
			return IsBuiltinSymbolLikeRecurser(program, typeChecker, constraint, predicate)
		}

		return false
	}

	predicateResult := predicate(t)
	if predicateResult == builtinPredicateMatches_True {
		return true
	} else if predicateResult == builtinPredicateMatches_False {
		return false
	}

	symbol := t.Symbol()
	if symbol != nil && symbol.Flags()&(ast.SymbolFlagsClass|ast.SymbolFlagsInterface) != 0 {
		declaredType := typeChecker.GetDeclaredTypeOfSymbol(symbol)
		for _, baseType := range typeChecker.GetBaseTypes(declaredType) {
			if IsBuiltinSymbolLikeRecurser(program, typeChecker, baseType, predicate) {
				return true
			}
		}
	}
	return false
}

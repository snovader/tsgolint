package no_generated_empty_object_type

import (
	"github.com/microsoft/TypeScript/tsc/shim/ast"
	"github.com/microsoft/TypeScript/tsc/shim/checker"
	"github.com/typescript-eslint/tsgolint/internal/rule"
	"github.com/typescript-eslint/tsgolint/internal/utils"
)

var NoGeneratedEmptyObjectTypeRule = rule.Rule{
	Name: "no-generated-empty-object-type",
	Run: func(ctx rule.RuleContext, options any) rule.RuleListeners {
		isEmptyObjectType := func(t *checker.Type) bool {
			return utils.IsObjectType(t) &&
				t.ObjectFlags()&checker.ObjectFlagsClassOrInterface == 0 &&
				len(ctx.TypeChecker.GetPropertiesOfType(t)) == 0 &&
				len(ctx.TypeChecker.GetIndexInfosOfType(t)) == 0 &&
				len(ctx.TypeChecker.GetSignaturesOfType(t, checker.SignatureKindCall)) == 0 &&
				len(ctx.TypeChecker.GetSignaturesOfType(t, checker.SignatureKindConstruct)) == 0 &&
				// Unresolved mapped types can have no members and accept number,
				// but reject string. Both primitives must be assignable to {}.
				ctx.TypeChecker.IsTypeAssignableTo(ctx.TypeChecker.GetNumberType(), t) &&
				ctx.TypeChecker.IsTypeAssignableTo(ctx.TypeChecker.GetStringType(), t)
		}
		checkNode := func(node *ast.Node) {
			for _, t := range utils.UnionTypeParts(ctx.TypeChecker.GetTypeAtLocation(node)) {
				if isEmptyObjectType(t) {
					ctx.ReportNode(node, rule.RuleMessage{
						Id:          "noGeneratedEmptyObjectType",
						Description: "This type resolves to `{}`, the empty object type. This was likely not intentional.",
					})
					return
				}
			}
		}
		return rule.RuleListeners{
			ast.KindIntersectionType: checkNode,
			ast.KindTypeReference: func(node *ast.Node) {
				parent := node.Parent
				for parent.Kind == ast.KindParenthesizedType {
					parent = parent.Parent
				}
				if node.AsTypeReferenceNode().TypeArguments != nil && parent.Kind != ast.KindIntersectionType {
					checkNode(node)
				}
			},
		}
	},
}

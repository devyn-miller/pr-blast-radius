package astparse

import (
	"context"
	"strings"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonParser_ExtractsSymbols(t *testing.T) {
	parser, err := NewPythonParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/python/discount_engine.py")
	fc, err := parser.Parse(context.Background(), "discount_engine.py", source)
	require.NoError(t, err)

	byName := make(map[string]SymbolCapture)
	for _, s := range fc.Symbols {
		byName[s.Name] = s
	}

	require.Contains(t, byName, "DiscountEngine")
	assert.Equal(t, KindClass, byName["DiscountEngine"].Kind)

	require.Contains(t, byName, "apply_discount")
	assert.Equal(t, KindMethod, byName["apply_discount"].Kind)
	assert.Equal(t, "DiscountEngine", byName["apply_discount"].QualifiedHint)
	assert.True(t, byName["apply_discount"].Exported)

	require.Contains(t, byName, "OrderService")
	assert.Equal(t, KindClass, byName["OrderService"].Kind)

	require.Contains(t, byName, "reserve_stock")
	assert.Equal(t, "OrderService", byName["reserve_stock"].QualifiedHint)

	require.Contains(t, byName, "create_order")
	assert.Equal(t, KindFunction, byName["create_order"].Kind)
}

func TestPythonParser_DecoratedRouteHandler_ProducesTag(t *testing.T) {
	parser, err := NewPythonParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/python/discount_engine.py")
	fc, err := parser.Parse(context.Background(), "discount_engine.py", source)
	require.NoError(t, err)

	var createOrder *SymbolCapture
	for i := range fc.Symbols {
		if fc.Symbols[i].Name == "create_order" {
			createOrder = &fc.Symbols[i]
		}
	}

	require.NotNil(t, createOrder)
	require.Len(t, createOrder.DecoratorTags, 1)
	assert.Equal(t, "route:POST:/orders", createOrder.DecoratorTags[0])
}

func TestPythonParser_ExtractsImports(t *testing.T) {
	parser, err := NewPythonParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/python/discount_engine.py")
	fc, err := parser.Parse(context.Background(), "discount_engine.py", source)
	require.NoError(t, err)

	paths := make([]string, 0, len(fc.Imports))
	for _, imp := range fc.Imports {
		paths = append(paths, imp.ModulePath)
	}

	assert.Contains(t, paths, "shared.money")
	assert.Contains(t, paths, "services.inventory_service")
	assert.Contains(t, paths, "flask")
}

func TestPythonParser_ExtractsCalls_ChainedSelfAttribute(t *testing.T) {
	parser, err := NewPythonParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/python/discount_engine.py")
	fc, err := parser.Parse(context.Background(), "discount_engine.py", source)
	require.NoError(t, err)

	var found struct {
		chainedApplyDiscount bool // self.engine.apply_discount(...)
		chainedReserve       bool // self.inventory.reserve(...)
		directRoundCurrency  bool // round_currency(...)
		selectorPrice        bool // svc.price(...)
	}

	for _, c := range fc.Calls {
		switch {
		case c.Kind == CallChained && c.MethodName == "apply_discount":
			found.chainedApplyDiscount = true
		case c.Kind == CallChained && c.MethodName == "reserve":
			found.chainedReserve = true
		case c.Kind == CallDirect && c.DirectName == "round_currency":
			found.directRoundCurrency = true
		case c.Kind == CallSelector && c.OperandName == "svc" && c.MethodName == "price":
			found.selectorPrice = true
		}
	}

	assert.True(t, found.chainedApplyDiscount, "expected self.engine.apply_discount(...) to be captured as Chained")
	assert.True(t, found.chainedReserve, "expected self.inventory.reserve(...) to be captured as Chained")
	assert.True(t, found.directRoundCurrency, "expected round_currency(...) to be captured as Direct")
	assert.True(t, found.selectorPrice, "expected svc.price(...) to be captured as Selector")
}

func TestPythonParser_ExtractsTypeHints_ParamAndConstructor(t *testing.T) {
	parser, err := NewPythonParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/python/discount_engine.py")
	fc, err := parser.Parse(context.Background(), "discount_engine.py", source)
	require.NoError(t, err)

	byVar := make(map[string]TypeHintCapture)
	for _, h := range fc.TypeHints {
		byVar[h.VarName] = h
	}

	require.Contains(t, byVar, "inventory")
	assert.Equal(t, "InventoryService", byVar["inventory"].ResolvedType)
	assert.Equal(t, InferFromParamType, byVar["inventory"].InferenceSource)

	require.Contains(t, byVar, "engine")
	assert.Equal(t, "DiscountEngine", byVar["engine"].ResolvedType)
	assert.Equal(t, InferFromConstructor, byVar["engine"].InferenceSource)

	require.Contains(t, byVar, "self")
	assert.Equal(t, InferFromSelfParam, byVar["self"].InferenceSource)
}

func TestDebug_PrintPythonTree(t *testing.T) {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())
	source := loadFixture(t, "testdata/python/discount_engine.py")
	tree, err := parser.ParseCtx(context.Background(), nil, source)
	require.NoError(t, err)
	defer tree.Close()
	t.Log(tree.RootNode().String())
}
func selfAttrHints(t *testing.T, src string) map[string]TypeHintCapture {
	t.Helper()
	parser, err := NewPythonParser()
	require.NoError(t, err)
	fc, err := parser.Parse(context.Background(), "svc.py", []byte(src))
	require.NoError(t, err)
	out := map[string]TypeHintCapture{}
	for _, h := range fc.TypeHints {
		if strings.HasPrefix(h.VarName, "self.") {
			out[h.VarName] = h
		}
	}
	return out
}

func TestPythonParser_SelfAttrHints_FromTypedParamAndConstructor(t *testing.T) {
	hints := selfAttrHints(t, `
class OrderService:
    def __init__(self, pricing: DiscountEngine):
        self.pricing = pricing
        self.inventory = InventoryService()
`)

	require.Contains(t, hints, "self.pricing")
	assert.Equal(t, "DiscountEngine", hints["self.pricing"].ResolvedType)
	assert.Equal(t, InferFromParamType, hints["self.pricing"].InferenceSource)

	require.Contains(t, hints, "self.inventory")
	assert.Equal(t, "InventoryService", hints["self.inventory"].ResolvedType)
	assert.Equal(t, InferFromConstructor, hints["self.inventory"].InferenceSource)
}

func TestPythonParser_SelfAttrHints_UndeterminedTypesProduceNoHint(t *testing.T) {
	hints := selfAttrHints(t, `
class A:
    def __init__(self, raw, other):
        self.untyped = raw
        self.engine = DiscountEngine()
        other.engine = LegacyPricer()

class B:
    def __init__(self):
        self.engine = LegacyPricer()
`)

	assert.NotContains(t, hints, "self.untyped", "untyped source variable: type unknown")
	assert.NotContains(t, hints, "self.engine", "conflicting types for self.engine in one file")
	assert.NotContains(t, hints, "other.engine", "only attributes of self are tracked")
	assert.Empty(t, hints)
}

func TestPythonParser_ChainedSelfCall_OperandMatchesSelfAttrHintKey(t *testing.T) {
	parser, err := NewPythonParser()
	require.NoError(t, err)
	fc, err := parser.Parse(context.Background(), "svc.py", []byte(`
class OrderService:
    def __init__(self, pricing: DiscountEngine):
        self.pricing = pricing

    def price(self, base, tier):
        return self.pricing.apply_discount(base, tier)
`))
	require.NoError(t, err)

	var chained *CallCapture
	for i := range fc.Calls {
		if fc.Calls[i].MethodName == "apply_discount" {
			chained = &fc.Calls[i]
		}
	}
	require.NotNil(t, chained)
	assert.Equal(t, CallChained, chained.Kind)
	assert.Equal(t, "self.pricing", chained.OperandName, "must equal the hint key the resolver looks up")
}

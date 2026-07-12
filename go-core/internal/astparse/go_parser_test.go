package astparse

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestGoParser_ExtractsSymbols(t *testing.T) {
	parser, err := NewGoParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/go/order_service.go")
	fc, err := parser.Parse(context.Background(), "order_service.go", source)
	require.NoError(t, err)

	names := make(map[string]SymbolCapture)
	for _, s := range fc.Symbols {
		names[s.Name] = s
	}

	require.Contains(t, names, "NewOrderService")
	assert.Equal(t, KindFunction, names["NewOrderService"].Kind)
	assert.True(t, names["NewOrderService"].Exported)

	require.Contains(t, names, "Create")
	assert.Equal(t, KindMethod, names["Create"].Kind)
	assert.Equal(t, "OrderService", names["Create"].QualifiedHint) // receiver type via §method.receiver_type
	assert.True(t, names["Create"].Exported)

	require.Contains(t, names, "notify")
	assert.False(t, names["notify"].Exported) // lowercase -> unexported, per Go convention

	require.Contains(t, names, "Notifier")
	assert.Equal(t, KindInterface, names["Notifier"].Kind)

	require.Contains(t, names, "OrderService")
	assert.Equal(t, KindClass, names["OrderService"].Kind)
}

func TestGoParser_ExtractsImports(t *testing.T) {
	parser, err := NewGoParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/go/order_service.go")
	fc, err := parser.Parse(context.Background(), "order_service.go", source)
	require.NoError(t, err)

	paths := make([]string, len(fc.Imports))
	for i, imp := range fc.Imports {
		paths[i] = imp.ModulePath
	}

	assert.Contains(t, paths, "fmt")
	assert.Contains(t, paths, "github.com/tinyshop/shared")
}

func TestGoParser_ExtractsCalls_SelectorAndDirect(t *testing.T) {
	parser, err := NewGoParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/go/order_service.go")
	fc, err := parser.Parse(context.Background(), "order_service.go", source)
	require.NoError(t, err)

	var found struct {
		applyDiscountSelector bool
		roundCurrencySelector bool
		newOrderServiceDirect bool
	}

	for _, c := range fc.Calls {
		switch {
		case c.Kind == CallSelector && c.OperandName == "s" && c.MethodName == "notify":
			// s.notify(orderID) inside Create
		case c.Kind == CallChained && c.MethodName == "ApplyDiscount":
			found.applyDiscountSelector = true
		case c.Kind == CallSelector && c.MethodName == "RoundCurrency":
			found.roundCurrencySelector = true
		case c.Kind == CallDirect && c.DirectName == "NewOrderService":
			found.newOrderServiceDirect = true
		}
	}

	assert.True(t, found.applyDiscountSelector, "expected s.pricing.ApplyDiscount(...) call to be captured")
	assert.True(t, found.newOrderServiceDirect, "expected NewOrderService(...) direct call to be captured")
}

func TestGoParser_ExtractsTypeHints_ConstructorAssignment(t *testing.T) {
	parser, err := NewGoParser()
	require.NoError(t, err)

	source := loadFixture(t, "testdata/go/order_service.go")
	fc, err := parser.Parse(context.Background(), "order_service.go", source)
	require.NoError(t, err)

	var svcHint *TypeHintCapture
	for i := range fc.TypeHints {
		if fc.TypeHints[i].VarName == "svc" {
			svcHint = &fc.TypeHints[i]
		}
	}

	require.NotNil(t, svcHint, "expected svc := NewOrderService(nil) to produce a TypeHintCapture")
	assert.Equal(t, InferFromConstructor, svcHint.InferenceSource)
	assert.Equal(t, "NewOrderService", svcHint.ResolvedType)
}
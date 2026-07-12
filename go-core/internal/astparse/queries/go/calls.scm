; Case 1: direct call — foo(...)
(call_expression
  function: (identifier) @call.direct_name) @call.site

; Case 2: selector call — pkg.Foo(...) OR receiver.Method(...)
(call_expression
  function: (selector_expression
    operand: (identifier) @call.operand
    field: (field_identifier) @call.method_name)) @call.site

; Case 3: chained selector — a.b.Method(...)
(call_expression
  function: (selector_expression
    operand: (selector_expression) @call.operand_chain
    field: (field_identifier) @call.method_name)) @call.site

; Case 4: function value used as callback / passed by reference (not invoked here,
; but this IS a dependency edge we don't want to silently miss)
(argument_list
  (identifier) @call.passed_as_value)

; Case 5: reflection-based dispatch — explicit red flag
(call_expression
  function: (selector_expression
    operand: (call_expression
      function: (selector_expression
        operand: (identifier) @reflect.pkg (#eq? @reflect.pkg "reflect")))
    field: (field_identifier) @reflect.method
    (#any-of? @reflect.method "MethodByName" "Call"))) @call.reflection_site
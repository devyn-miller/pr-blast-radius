; Direct call — foo(...)
(call
  function: (identifier) @call.direct_name) @call.site

; Attribute call — obj.method(...) / self.method(...)
(call
  function: (attribute
    object: (identifier) @call.operand
    attribute: (identifier) @call.method_name)) @call.site

; Chained attribute call — a.b.c(...)
(call
  function: (attribute
    object: (attribute) @call.operand_chain
    attribute: (identifier) @call.method_name)) @call.site

; Dynamic dispatch: getattr(obj, "method_name")(...)
(call
  function: (call
    function: (identifier) @dyn.fn (#eq? @dyn.fn "getattr")
    arguments: (argument_list
      (identifier) @dyn.target
      (string) @dyn.attr_name_literal))) @call.dynamic_getattr_site

; Function passed as a value (callback, decorator factory arg, event handler registration)
(argument_list
  (identifier) @call.passed_as_value)
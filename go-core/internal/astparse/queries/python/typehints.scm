; Type-hinted parameter: def handler(svc: OrderService):
(parameter
  (identifier) @var.name
  type: (type (identifier) @var.type))

; Constructor assignment: svc = OrderService(...)
(assignment
  left: (identifier) @var.name
  right: (call
    function: (identifier) @var.constructor_name))

; self-parameter of the enclosing method, used to resolve self.foo()
(function_definition
  parameters: (parameters . (identifier) @self.param_name (#eq? @self.param_name "self")))
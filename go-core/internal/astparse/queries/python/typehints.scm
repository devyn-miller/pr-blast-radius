; Type-hinted parameter: def handler(svc: OrderService):
(parameter
  (identifier) @var.name
  type: (type (identifier) @var.type))

; Constructor assignment: svc = OrderService(...)
(assignment
  left: (identifier) @var.name
  right: (call
    function: (identifier) @var.constructor_name))

; Instance attribute from a constructor: self.inventory = InventoryService()
; Tree: (assignment left: (attribute object: (identifier) attribute: (identifier))
;                   right: (call function: (identifier) ...))
(assignment
  left: (attribute
    object: (identifier) @self_attr.object (#eq? @self_attr.object "self")
    attribute: (identifier) @self_attr.name)
  right: (call
    function: (identifier) @self_attr.constructor_name))

; Instance attribute copied from a variable: self.pricing = pricing
; Tree: (assignment left: (attribute ...) right: (identifier)). The variable's
; type is looked up from the file's param-typed hints by the parser.
(assignment
  left: (attribute
    object: (identifier) @self_attr.object (#eq? @self_attr.object "self")
    attribute: (identifier) @self_attr.name)
  right: (identifier) @self_attr.source_var)

; self-parameter of the enclosing method, used to resolve self.foo()
(function_definition
  parameters: (parameters . (identifier) @self.param_name (#eq? @self.param_name "self")))
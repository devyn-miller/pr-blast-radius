; Parameter with explicit type: func f(svc *OrderService)
(parameter_declaration
  name: (identifier) @var.name
  type: [
    (pointer_type (type_identifier) @var.type)
    (type_identifier) @var.type
  ])

; Short var decl via constructor call: svc := NewOrderService()
(short_var_declaration
  left: (expression_list (identifier) @var.name)
  right: (expression_list
    (call_expression
      function: (identifier) @var.constructor_name)))

; var decl with explicit type: var svc OrderService
(var_declaration
  (var_spec
    name: (identifier) @var.name
    type: (type_identifier) @var.type))
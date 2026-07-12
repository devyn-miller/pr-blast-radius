; --- Top-level functions ---
(function_declaration
  name: (identifier) @function.name) @function.definition

; --- Methods (has a receiver -> tells us the owning type) ---
(method_declaration
  receiver: (parameter_list
    (parameter_declaration
      type: [
        (pointer_type (type_identifier) @method.receiver_type)
        (type_identifier) @method.receiver_type
      ]))
  name: (field_identifier) @method.name) @method.definition

; --- Struct type declarations ---
(type_declaration
  (type_spec
    name: (type_identifier) @type.name
    type: (struct_type) @struct.body)) @type.definition.struct

; --- Interface type declarations + their method sets ---
(type_declaration
  (type_spec
    name: (type_identifier) @interface.name
    type: (interface_type
      (method_elem
        name: (field_identifier) @interface.method_name)*))) @type.definition.interface
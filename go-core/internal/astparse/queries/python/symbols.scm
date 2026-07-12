; --- Module-level functions ---
(function_definition
  name: (identifier) @function.name) @function.definition

; --- Classes ---
(class_definition
  name: (identifier) @class.name
  superclasses: (argument_list)? @class.bases
  body: (block) @class.body) @class.definition

; --- Methods (function_definition nested directly in a class body) ---
(class_definition
  name: (identifier) @class.name
  body: (block
    (function_definition
      name: (identifier) @method.name) @method.definition))

; --- Decorated definitions (route handlers, etc.) ---
(decorated_definition
  (decorator
    (call
      function: (attribute
        object: (identifier) @decorator.object
        attribute: (identifier) @decorator.attribute)
      arguments: (argument_list . (string) @decorator.route_path))) @decorator.call
  definition: (function_definition
    name: (identifier) @decorated.function_name))
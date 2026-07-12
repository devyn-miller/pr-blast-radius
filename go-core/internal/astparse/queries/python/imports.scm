(import_statement
  name: (dotted_name) @import.module) @import.site

(import_statement
  name: (aliased_import
    name: (dotted_name) @import.module
    alias: (identifier) @import.alias)) @import.site

(import_from_statement
  module_name: (dotted_name) @import.module
  name: (dotted_name) @import.symbol) @import.site

; Dynamic import — importlib.import_module("some.module") — flag, don't resolve
(call
  function: (attribute
    object: (identifier) @dyn.obj (#eq? @dyn.obj "importlib")
    attribute: (identifier) @dyn.method (#eq? @dyn.method "import_module"))
  arguments: (argument_list . (string)? @dyn.literal_module)) @import.dynamic_site
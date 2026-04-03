package engine

// registerResource registers the builtin Resource and Entity object types.
//
//   - Object/Resource has field kind:String
//   - Object/Resource/Entity inherits kind from Resource and adds spec:String, entity:String
//
// These are registered via installDef so they get proper handler resolution
// and can be referenced by name in AQL code (e.g. make Entity {...}).
func registerResource(r *Registry) {
	// --- Resource: {kind:String} ---
	resourceFields := NewOrderedMap()
	resourceFields.Set("kind", NewTypeLiteral(TString))

	resourceInfo := ObjectTypeInfo{
		Fields: resourceFields,
		Parent: nil,
		ID:     formatFixedTypeID("Object/Resource", builtinTypeIDs["Object/Resource"]),
	}

	installDef(r, "Resource", NewObjectType(resourceInfo))

	// Retrieve the installed Resource type so Entity can reference it as parent.
	resourceVal := r.DefStacks["Resource"][len(r.DefStacks["Resource"])-1]
	installedResource, _ := resourceVal.AsObjectType()

	// --- Entity: {spec:String, entity:String} inherits Resource ---
	entityFields := NewOrderedMap()
	entityFields.Set("spec", NewTypeLiteral(TString))
	entityFields.Set("entity", NewTypeLiteral(TString))

	entityInfo := ObjectTypeInfo{
		Fields: entityFields,
		Parent: &installedResource,
		ID:     formatFixedTypeID("Object/Resource/Entity", builtinTypeIDs["Object/Resource/Entity"]),
	}

	installDef(r, "Entity", NewObjectType(entityInfo))
}

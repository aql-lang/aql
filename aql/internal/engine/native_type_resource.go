package engine

// RegisterResource registers the builtin Resource and Entity object types.
//
//   - Object/Resource has field kind:String
//   - Object/Resource/Entity inherits kind from Resource and adds spec:String, entity:String
//
// These are registered via InstallDef so they get proper handler resolution
// and can be referenced by name in AQL code (e.g. make Entity {...}).
func RegisterResource(r *Registry) {
	// --- Resource: {kind:String} ---
	resourceFields := NewOrderedMap()
	resourceFields.Set("kind", NewTypeLiteral(TString))

	resourceInfo := ObjectTypeInfo{
		Fields: resourceFields,
		Parent: nil,
		ID:     FormatFixedTypeID("Object/Resource", BuiltinTypeIDs["Object/Resource"]),
	}

	InstallDef(r, "Resource", NewObjectType(resourceInfo))

	// Retrieve the installed Resource type so Entity can reference it as parent.
	resourceVal, _ := r.TopOfDefStack("Resource")
	installedResource, _ := resourceVal.AsObjectType()

	// --- Entity: {spec:String, entity:String} inherits Resource ---
	entityFields := NewOrderedMap()
	entityFields.Set("spec", NewTypeLiteral(TString))
	entityFields.Set("entity", NewTypeLiteral(TString))

	entityInfo := ObjectTypeInfo{
		Fields: entityFields,
		Parent: &installedResource,
		ID:     FormatFixedTypeID("Object/Resource/Entity", BuiltinTypeIDs["Object/Resource/Entity"]),
	}

	InstallDef(r, "Entity", NewObjectType(entityInfo))
}

package schema

func (s *SchemaDefinition) FindField(name string) *FieldDefinition{
	for _, field := range s.Fields {
		if field.Name == name {
			return field
		}
	}
	return nil
}

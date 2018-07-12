package models

// CreateOrUpdateList updates a list or creates it if it doesn't exist
func CreateOrUpdateList(list *List) (err error) {

	if list.ID == 0 {
		_, err = x.Insert(list)
	} else {
		_, err = x.ID(list.ID).Update(list)
	}

	if err != nil {
		return
	}

	*list, err = GetListByID(list.ID)

	return

}

// Update implements the update method of CRUDable
func (l *List) Update(id int64) (err error) {
	l.ID = id

	// Check if it exists
	_, err = GetListByID(l.ID)
	if err != nil {
		return
	}

	return CreateOrUpdateList(l)
}

// Create implements the create method of CRUDable
func (l *List) Create(doer *User, id int64) (err error) {
	// Check rights
	user, _, err := GetUserByID(doer.ID)
	if err != nil {
		return
	}

	l.Owner.ID = user.ID
	l.ID = 0 // Otherwise only the first time a new list would be created

	return CreateOrUpdateList(l)
}

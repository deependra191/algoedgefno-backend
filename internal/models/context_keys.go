package models

// UserIDKey is the gin.Context key under which an authenticated user's
// uuid.UUID identity is stored. Middleware sets it (PR 2 onward); handlers
// read it via the extractUserID helper. Lives in models so both layers
// reference the same constant without cross-component import.
const UserIDKey = "userID"

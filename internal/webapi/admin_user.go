package webapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	usertempl "geevly/internal/webapi/templates/admin/user"
	components "geevly/internal/webapi/templates/components"

	"github.com/clerkinc/clerk-sdk-go/clerk"
	"github.com/go-chi/chi/v5"
)

func (s *Server) userAdminRouter(r chi.Router) {
	r.Get("/", s.adminListUsers)
	r.Get("/create", s.adminCreateUserForm)
	r.Post(`/create`, s.adminCreateUser)

	r.Group(func(r chi.Router) {
		r.Use(s.setUserIDMiddleware)
		r.Get(`/{ID}`, s.adminViewUser)
		r.Post(`/{ID}`, s.adminUpdateUser)
		r.Get(`/{ID}/history`, s.adminUserHistory)
		r.Put(`/{ID}/setRole`, s.setUserRole)
	})
}

func (s *Server) setUserIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "ID")
		ctx := context.WithValue(r.Context(), "userID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) getUserIDFromContext(ctx context.Context) string {
	id, ok := ctx.Value("userID").(string)
	if !ok {
		// acceptable to be an internal panic because this should not be called unless the
		// middleware was called.
		panic("No user ID in context")
	}
	return id
}

func (s *Server) adminViewUser(w http.ResponseWriter, r *http.Request) {
	// Get the user ID from the context
	userID := s.getUserIDFromContext(r.Context())

	// Get the user from Clerk
	clerkUser, err := s.Clerk.Users().Read(userID)
	if err != nil {
		s.errorPage(w, r, "Error fetching user", err)
		return
	}

	isAdmin, err := getMetadataValue[bool](clerkUser.PrivateMetadata, "admin")
	if err != nil {
		isAdmin = false
	}

	// Convert Clerk user to our internal user model
	user := &usertempl.ViewParams{
		ID:        userID,
		FirstName: *clerkUser.FirstName,
		LastName:  *clerkUser.LastName,
		Username:  *clerkUser.Username,
		Active:    !clerkUser.Banned,
		IsAdmin:   isAdmin,
	}

	// Render the user view template
	s.renderTempl(w, r, usertempl.View(*user)) // Using 1 as a placeholder for version
}

func (s *Server) setUserRole(w http.ResponseWriter, r *http.Request) {
	userID := s.getUserIDFromContext(r.Context())
	role := r.URL.Query().Get("role")

	// Get the user from Clerk
	clerkUser, err := s.Clerk.Users().Read(userID)
	if err != nil {
		s.errorPage(w, r, "Error fetching user", err)
		return
	}

	value := r.URL.Query().Get("value")
	valueBool, err := strconv.ParseBool(value)
	if err != nil {
		s.errorPage(w, r, "Error parsing value", err)
		return
	}

	privateMetadata := clerkUser.PrivateMetadata
	switch role {
	case "system_admin":
		privateMetadata, err = setMetadataValue(privateMetadata, "admin", valueBool)
		if err != nil {
			s.errorPage(w, r, "Error setting metadata", err)
			return
		}
	case "active":
		if valueBool {
			s.Clerk.Users().Unban(userID)
		} else {
			s.Clerk.Users().Ban(userID)
		}
	}

	// Update the user in Clerk
	_, err = s.Clerk.Users().Update(userID, &clerk.UpdateUser{
		PrivateMetadata: privateMetadata,
	})
	if err != nil {
		s.errorPage(w, r, "Error updating user", err)
		return
	}

	// Redirect to the view user page
	http.Redirect(w, r, fmt.Sprintf("/admin/user/%s", userID), http.StatusSeeOther)
}

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request) {
	page := int(s.pageQuery(r))
	limit := int(s.limitQuery(r))
	offset := (page - 1) * limit

	// Get the list of users from Clerk
	params := clerk.ListAllUsersParams{
		Limit:  &limit,
		Offset: &offset,
	}
	clerkUsers, err := s.Clerk.Users().ListAll(params)
	if err != nil {
		s.errorPage(w, r, "Error listing users", err)
		return
	}

	// Convert Clerk users to our internal user model
	users := make([]usertempl.User, len(clerkUsers))
	for i, cu := range clerkUsers {
		users[i] = usertempl.User{
			ID:       cu.ID,
			Username: *cu.Username,
			Active:   !cu.Banned,
			Name:     *cu.FirstName + " " + *cu.LastName,
		}
	}

	// // Get total count for pagination
	totalCount, err := s.Clerk.Users().Count(params)
	if err != nil {
		s.errorPage(w, r, "Error getting user count", err)
		return
	}

	totalCountInt := int(totalCount.TotalCount)
	// Create pagination object
	pagination := components.NewPagination(uint(page), uint(limit), uint(totalCountInt))

	// Create ListResponse
	response := &usertempl.ListResponse{
		Users:      users,
		Pagination: pagination,
	}

	s.renderTempl(w, r, usertempl.List(response, pagination))
}

func (s *Server) adminCreateUserForm(w http.ResponseWriter, r *http.Request) {
	s.renderTempl(w, r, usertempl.Create())

}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	err := r.ParseForm()
	if err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	// Extract user details from form
	firstName := r.FormValue("first_name")
	lastName := r.FormValue("last_name")
	username := r.FormValue("username")
	password := r.FormValue("password")

	// Create user params
	params := clerk.CreateUserParams{
		Username:  &username,
		FirstName: &firstName,
		LastName:  &lastName,
		Password:  &password,
	}

	// Create user in Clerk
	_, err = s.Clerk.Users().Create(params)
	if err != nil {
		s.errorPage(w, r, "Error creating user", err)
		return
	}

	// Redirect to user list page
	http.Redirect(w, r, "/admin/user", http.StatusSeeOther)
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	// Parse form data
	err := r.ParseForm()
	if err != nil {
		s.errorPage(w, r, "Error parsing form", err)
		return
	}

	userID := s.getUserIDFromContext(r.Context())

	// Extract updated user details from form
	firstName := r.FormValue("first_name")
	lastName := r.FormValue("last_name")
	username := r.FormValue("username")

	// Create update params
	params := clerk.UpdateUser{
		FirstName: &firstName,
		LastName:  &lastName,
		Username:  &username,
	}

	// Update user in Clerk
	_, err = s.Clerk.Users().Update(userID, &params)
	if err != nil {
		s.errorPage(w, r, "Error updating user", err)
		return
	}

	// Redirect back to the user's view page
	http.Redirect(w, r, fmt.Sprintf("/admin/user/%s", userID), http.StatusSeeOther)
}

func (s *Server) toggleUserStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: toggle user status
}

func (s *Server) adminUserHistory(w http.ResponseWriter, r *http.Request) {
	// TODO: get user history
}

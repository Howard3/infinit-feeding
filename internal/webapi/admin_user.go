package webapi

import (
	"context"
	"net/http"

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
		r.Put(`/{ID}/toggleStatus`, s.toggleUserStatus)

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

	// Convert Clerk user to our internal user model
	user := &usertempl.ViewParams{
		ID:        userID,
		FirstName: *clerkUser.FirstName,
		LastName:  *clerkUser.LastName,
		Email:     clerkUser.EmailAddresses[0].EmailAddress,
		Active:    !clerkUser.Banned,
	}

	// Render the user view template
	s.renderTempl(w, r, usertempl.View(*user)) // Using 1 as a placeholder for version
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
			ID:     cu.ID,
			Email:  cu.EmailAddresses[0].EmailAddress,
			Active: !cu.Banned,
			Name:   *cu.FirstName + " " + *cu.LastName,
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
	// TODO: create user
}

func (s *Server) adminUpdateUser(w http.ResponseWriter, r *http.Request) {
	// TODO: update user
}

func (s *Server) toggleUserStatus(w http.ResponseWriter, r *http.Request) {
	// TODO: toggle user status
}

func (s *Server) adminUserHistory(w http.ResponseWriter, r *http.Request) {
	// TODO: get user history
}

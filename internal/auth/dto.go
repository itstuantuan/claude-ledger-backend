package auth

type LoginRequest struct {
	Account  string `json:"account" validate:"required,max=80"`
	Password string `json:"password" validate:"required,max=128"`
}

type UserResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Account     string   `json:"account"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
	Status      string   `json:"status"`
}

type SessionResponse struct {
	AccessToken string       `json:"accessToken"`
	ExpiresIn   int64        `json:"expiresIn"`
	User        UserResponse `json:"user"`
}

func userResponse(user User) UserResponse {
	return UserResponse{ID: user.ID.String(), Name: user.Name, Account: user.Account, Role: user.Role, Permissions: user.Permissions, Status: user.Status}
}

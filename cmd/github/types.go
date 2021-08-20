package github

type options struct {
	Users           *[]string
	Emails          *[]string
	Repos           *[]string
	Fusers          *string
	Femails         *string
	Frepos          *string
	Lookup          *bool
	Forks           *bool
	Members         *bool
	Pulls           *bool
	Contributors    *bool
	List            *bool
	Rate            *bool
	Search          *bool
	MaxPullRequests *int
	Threads         *int
	Token           *string
	BaseURL         *string
	UploadURL       *string
}

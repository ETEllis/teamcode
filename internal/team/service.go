package team

type Service struct {
	Context *TeamContextService
	Board   *TaskBoardService
	Handoff *HandoffService
	Inbox   *InboxService
	Members *MemberService
}

func NewService() *Service {
	sharedStore := newStore("")
	return NewServiceWithStore(sharedStore)
}

func NewServiceWithBaseDir(baseDir string) *Service {
	return NewServiceWithStore(newStore(baseDir))
}

func NewServiceWithStore(sharedStore *store) *Service {
	return &Service{
		Context: NewTeamContextService(sharedStore),
		Board:   NewTaskBoardService(sharedStore),
		Handoff: NewHandoffService(sharedStore),
		Inbox:   NewInboxService(sharedStore),
		Members: NewMemberService(sharedStore),
	}
}

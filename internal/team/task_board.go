package team

import (
	"context"
	"time"
)

type TaskBoard struct {
	TeamName    string              `json:"teamName"`
	Columns     map[string][]string `json:"columns"`
	Assignments map[string]string   `json:"assignments"`
	Constraints map[string]any      `json:"constraints"`
}

type TaskBoardService struct {
	store *store
}

func NewTaskBoardService(sharedStore *store) *TaskBoardService {
	return &TaskBoardService{store: sharedStore}
}

func defaultBoard(teamName string) *TaskBoard {
	return &TaskBoard{
		TeamName: teamName,
		Columns: map[string][]string{
			"backlog":     {},
			"ready":       {},
			"in_progress": {},
			"in_review":   {},
			"done":        {},
			"blocked":     {},
		},
		Assignments: map[string]string{},
		Constraints: map[string]any{
			"maxWip":    3,
			"updatedAt": time.Now().UnixMilli(),
		},
	}
}

func (s *TaskBoardService) CreateBoard(ctx context.Context, teamName string) (*TaskBoard, error) {
	_ = ctx
	board, err := s.ReadBoard(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	if board == nil {
		board = defaultBoard(teamName)
		if err := s.store.writeJSON(teamName, "task_board.json", board); err != nil {
			return nil, err
		}
	}
	return board, nil
}

func (s *TaskBoardService) ReadBoard(ctx context.Context, teamName string) (*TaskBoard, error) {
	_ = ctx
	var board TaskBoard
	if err := s.store.readJSON(teamName, "task_board.json", &board); err != nil {
		return nil, err
	}
	if board.TeamName == "" {
		return nil, nil
	}
	return &board, nil
}

func (s *TaskBoardService) MoveTask(ctx context.Context, teamName, taskID, fromColumn, toColumn string, agent string) (*TaskBoard, error) {
	_ = ctx
	board, err := s.CreateBoard(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	if board.Columns == nil {
		board.Columns = defaultBoard(teamName).Columns
	}
	if fromColumn != "" {
		board.Columns[fromColumn] = removeTask(board.Columns[fromColumn], taskID)
	}
	for column, tasks := range board.Columns {
		if column != fromColumn {
			board.Columns[column] = removeTask(tasks, taskID)
		}
	}
	board.Columns[toColumn] = appendUnique(board.Columns[toColumn], taskID)
	if agent != "" {
		board.Assignments[taskID] = agent
	}
	board.Constraints["updatedAt"] = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "task_board.json", board); err != nil {
		return nil, err
	}
	return board, nil
}

func (s *TaskBoardService) AddTaskToColumn(ctx context.Context, teamName, taskID, column string) (*TaskBoard, error) {
	_ = ctx
	board, err := s.CreateBoard(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	if board.Columns == nil {
		board.Columns = defaultBoard(teamName).Columns
	}
	board.Columns[column] = appendUnique(board.Columns[column], taskID)
	board.Constraints["updatedAt"] = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "task_board.json", board); err != nil {
		return nil, err
	}
	return board, nil
}

func (s *TaskBoardService) AssignTask(ctx context.Context, teamName, taskID, agent string) (*TaskBoard, error) {
	_ = ctx
	board, err := s.CreateBoard(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	board.Assignments[taskID] = agent
	board.Constraints["updatedAt"] = time.Now().UnixMilli()
	if err := s.store.writeJSON(teamName, "task_board.json", board); err != nil {
		return nil, err
	}
	return board, nil
}

func (s *TaskBoardService) GetTaskLocation(ctx context.Context, teamName, taskID string) (string, error) {
	_ = ctx
	board, err := s.CreateBoard(context.Background(), teamName)
	if err != nil {
		return "", err
	}
	for column, tasks := range board.Columns {
		for _, task := range tasks {
			if task == taskID {
				return column, nil
			}
		}
	}
	return "", nil
}

func (s *TaskBoardService) GetBoardSummary(ctx context.Context, teamName string) (map[string]int, error) {
	_ = ctx
	board, err := s.CreateBoard(context.Background(), teamName)
	if err != nil {
		return nil, err
	}
	summary := make(map[string]int, len(board.Columns))
	for column, tasks := range board.Columns {
		summary[column] = len(tasks)
	}
	return summary, nil
}

func appendUnique(existing []string, taskID string) []string {
	for _, value := range existing {
		if value == taskID {
			return existing
		}
	}
	return append(existing, taskID)
}

func removeTask(existing []string, taskID string) []string {
	filtered := make([]string, 0, len(existing))
	for _, value := range existing {
		if value != taskID {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

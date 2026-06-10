package service

import (
	"edge5/internal/model"
	"edge5/internal/repository"

	"go.uber.org/zap"
)

type TaskService struct {
	repo   *repository.TaskRepository
	logger *zap.Logger
}

func NewTaskService(repo *repository.TaskRepository, logger *zap.Logger) *TaskService {
	return &TaskService{repo: repo, logger: logger}
}

func (s *TaskService) Create(task *model.Task) error {
	return s.repo.Create(task)
}

func (s *TaskService) Update(task *model.Task) error {
	return s.repo.Update(task)
}

func (s *TaskService) Delete(id uint64) error {
	return s.repo.Delete(id)
}

func (s *TaskService) DeleteBatch(ids []uint64) error {
	return s.repo.DeleteBatch(ids)
}

func (s *TaskService) GetByID(id uint64) (*model.Task, error) {
	return s.repo.GetByID(id)
}

func (s *TaskService) List(page, pageSize int, name string) ([]model.Task, int64, error) {
	return s.repo.List(page, pageSize, name)
}

// StartTask 开启任务
func (s *TaskService) StartTask(id uint64) error {
	task, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	task.State = true
	return s.repo.Update(task)
}

// StopTask 关闭任务
func (s *TaskService) StopTask(id uint64) error {
	task, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	task.State = false
	return s.repo.Update(task)
}

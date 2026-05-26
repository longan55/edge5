package captcha

import (
	"edge5/config"
	"sync"

	"github.com/mojocn/base64Captcha"
)

var (
	store  = NewMemoryStore()
	driver base64Captcha.Driver
)

type MemoryStore struct {
	sync.RWMutex
	data map[string]storeItem
}

type storeItem struct {
	value   string
	expiry  int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]storeItem),
	}
}

func (s *MemoryStore) Set(id string, value string) error {
	s.Lock()
	defer s.Unlock()

	expire := config.CONFIG.Captcha.ExpireTime
	if expire <= 0 {
		expire = 300
	}

	s.data[id] = storeItem{
		value:  value,
		expiry: int64(expire),
	}
	return nil
}

func (s *MemoryStore) Get(id string, clear bool) string {
	s.Lock()
	defer s.Unlock()

	item, ok := s.data[id]
	if !ok {
		return ""
	}

	if clear {
		delete(s.data, id)
	}

	return item.value
}

func (s *MemoryStore) Verify(id, answer string, clear bool) bool {
	v := s.Get(id, clear)
	return v != "" && v == answer
}

func InitCaptcha() {
	height := 80
	width := 240
	length := config.CONFIG.Captcha.CaptchaLength
	if length <= 0 {
		length = 4
	}

	driver = base64Captcha.NewDriverDigit(height, width, length, 0.7, 80)
}

func GenerateCaptcha() (id, base64 string, err error) {
	if driver == nil {
		InitCaptcha()
	}

	c := base64Captcha.NewCaptcha(driver, store)
	id, base64, err = c.Generate()
	return
}

func VerifyCaptcha(id, answer string) bool {
	return store.Verify(id, answer, true)
}

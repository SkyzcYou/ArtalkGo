package model

import (
	"github.com/ArtalkJS/ArtalkGo/lib"
	"gorm.io/gorm"
)

type CommentType string

const (
	CommentCollapsed CommentType = "collapsed"
	CommentPending   CommentType = "pengding"
	CommentDeleted   CommentType = "deleted"
)

type Comment struct {
	gorm.Model
	Content string

	UserID  uint   `gorm:"index"`
	PageKey string `gorm:"index"`
	User    User   `gorm:"foreignKey:UserID;references:ID"`
	Page    Page   `gorm:"foreignKey:PageKey;references:Key"`

	Rid  uint `gorm:"index"`
	UA   string
	IP   string
	Type CommentType
}

func (c Comment) IsEmpty() bool {
	return c.ID == 0
}

func (c *Comment) FetchUser() User {
	if !c.User.IsEmpty() {
		return c.User
	}

	// TODO: 先从 Redis 查询
	var user User
	lib.DB.First(&user, c.UserID)

	c.User = user
	return user
}

func (c *Comment) FetchPage() Page {
	if !c.Page.IsEmpty() {
		return c.Page
	}

	var page Page
	lib.DB.Where(&Page{Key: c.PageKey}).First(&page)

	c.Page = page
	return page
}

func (c Comment) FetchChildren(filter func(db *gorm.DB) *gorm.DB) []Comment {
	children := []Comment{}
	fetchChildrenOnce(&children, c, filter) // TODO: children 数量限制
	return children
}

func fetchChildrenOnce(src *[]Comment, parentComment Comment, filter func(db *gorm.DB) *gorm.DB) {
	children := []Comment{}
	lib.DB.Scopes(filter).Where("rid = ?", parentComment.ID).Order("created_at ASC").Find(&children)

	for _, child := range children {
		*src = append(*src, child)
		fetchChildrenOnce(src, child, filter) // loop
	}
}

type CookedComment struct {
	ID             uint   `json:"id"`
	Content        string `json:"content"`
	Nick           string `json:"nick"`
	EmailEncrypted string `json:"email_encrypted"`
	Link           string `json:"link"`
	UA             string `json:"ua"`
	Date           string `json:"date"`
	IsCollapsed    bool   `json:"is_collapsed"`
	IsPending      bool   `json:"is_pending"`
	Rid            uint   `json:"rid"`
}

func (c Comment) ToCooked() CookedComment {
	user := c.FetchUser()
	//page := c.FetchPage()

	return CookedComment{
		ID:             c.ID,
		Content:        c.Content,
		Nick:           user.Name,
		EmailEncrypted: lib.GetMD5Hash(user.Email),
		Link:           user.Link,
		UA:             c.UA,
		Date:           c.CreatedAt.Local().String(),
		IsCollapsed:    c.Type == CommentCollapsed,
		IsPending:      c.Type == CommentPending,
		Rid:            c.Rid,
	}
}
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"cloud.google.com/go/logging"
	. "github.com/go-jet/jet/v2/postgres"
	"github.com/go-jet/jet/v2/qrm"
	"github.com/google/uuid"
	"github.com/gosimple/slug"
	"github.com/marcusmonteirodesouza/realworld-backend-go-jet-postgresql/.gen/realworld/public/model"
	. "github.com/marcusmonteirodesouza/realworld-backend-go-jet-postgresql/.gen/realworld/public/table"
)

type ArticlesService struct {
	db           *sql.DB
	logger       *logging.Logger
	usersService *UsersService
}

func NewArticlesService(db *sql.DB, logger *logging.Logger, usersService *UsersService) ArticlesService {
	return ArticlesService{
		db:           db,
		logger:       logger,
		usersService: usersService,
	}
}

type CreateArticle struct {
	AuthorID    uuid.UUID
	Title       string
	Description string
	Body        string
	TagList     *[]string
}

func NewCreateArticle(authorId uuid.UUID, title string, description string, body string, tagList *[]string) CreateArticle {
	return CreateArticle{
		AuthorID:    authorId,
		Title:       title,
		Description: description,
		Body:        body,
		TagList:     tagList,
	}
}

type ListTags struct {
	ArticleID *uuid.UUID
}

func NewListTags(articleId *uuid.UUID) ListTags {
	return ListTags{
		ArticleID: articleId,
	}
}

func (articlesService *ArticlesService) CreateArticle(ctx context.Context, createArticle CreateArticle) (*model.Article, error) {
	if createArticleBytes, err := json.Marshal(createArticle); err == nil {
		articlesService.logger.StandardLogger(logging.Info).Printf("Creating article. %s", string(createArticleBytes))
	}

	author, err := articlesService.usersService.GetUserById(ctx, createArticle.AuthorID)
	if err != nil {
		return nil, err
	}

	slug := articlesService.makeSlug(author.Username, createArticle.Title)

	if err = articlesService.validateSlug(ctx, slug); err != nil {
		return nil, err
	}

	article := model.Article{
		AuthorID:    &author.ID,
		Slug:        slug,
		Title:       createArticle.Title,
		Description: createArticle.Description,
		Body:        createArticle.Body,
	}

	tx, err := articlesService.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	articleInsertStmt := Article.INSERT(Article.AuthorID, Article.Slug, Article.Title, Article.Description, Article.Body).MODEL(article).RETURNING(Article.AllColumns)

	if err = articleInsertStmt.QueryContext(ctx, tx, &article); err != nil {
		return nil, err
	}

	if createArticle.TagList != nil {
		for _, tagName := range *createArticle.TagList {
			tagName = articlesService.makeTagName(tagName)

			tag, err := articlesService.getTagByName(ctx, tagName)
			if err != nil {
				if _, ok := err.(*NotFoundError); ok {
					tagModel := model.ArticleTag{
						Name: tagName,
					}

					insertTagStmt := ArticleTag.INSERT(ArticleTag.Name).MODEL(tagModel).RETURNING(ArticleTag.AllColumns)

					if err = insertTagStmt.QueryContext(ctx, tx, &tagModel); err != nil {
						return nil, err
					}

					tag = &tagModel
				} else {
					return nil, err
				}
			}

			articleArticleTag := model.ArticleArticleTag{
				ArticleID:    &article.ID,
				ArticleTagID: &tag.ID,
			}

			insertArticleArticleTagStmt := ArticleArticleTag.INSERT(ArticleArticleTag.ArticleID, ArticleArticleTag.ArticleTagID).MODEL(articleArticleTag)

			if _, err = insertArticleArticleTagStmt.ExecContext(ctx, tx); err != nil {
				return nil, err
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, err
	}

	return &article, nil
}

func (articlesService *ArticlesService) ListTags(ctx context.Context, listTags ListTags) (*[]model.ArticleTag, error) {
	var tags []model.ArticleTag

	condition := Bool(true)

	if listTags.ArticleID != nil {
		condition = condition.AND(ArticleTag.ID.IN(ArticleArticleTag.SELECT(ArticleArticleTag.ArticleTagID).WHERE(ArticleArticleTag.ArticleID.EQ(UUID(listTags.ArticleID)))))
	}

	listTagsStmt := SELECT(ArticleTag.AllColumns).FROM(ArticleTag).WHERE(condition).ORDER_BY(ArticleTag.Name)

	err := listTagsStmt.QueryContext(ctx, articlesService.db, &tags)
	if err != nil {
		return nil, err
	}

	return &tags, nil
}

func (articlesService *ArticlesService) getTagByName(ctx context.Context, tagName string) (*model.ArticleTag, error) {
	var tag model.ArticleTag

	getTagByNameStmt := ArticleTag.SELECT(ArticleTag.AllColumns).WHERE(ArticleTag.Name.EQ(String(tagName)))

	err := getTagByNameStmt.QueryContext(ctx, articlesService.db, &tag)
	if err != nil {
		if errors.Is(err, qrm.ErrNoRows) {
			return nil, &NotFoundError{msg: fmt.Sprintf("Tag name %s not found", tagName)}
		}
		return nil, err
	}

	return &tag, nil
}

func (articlesService *ArticlesService) validateSlug(ctx context.Context, slug string) error {
	var slugExistsDest struct {
		SlugExists bool
	}

	slugExistsStmt := SELECT(EXISTS(Article.SELECT(Article.ID).WHERE(Article.Slug.EQ(String(slug)))).AS("slug_exists"))

	err := slugExistsStmt.QueryContext(ctx, articlesService.db, &slugExistsDest)
	if err != nil {
		return err
	}

	if slugExistsDest.SlugExists {
		return &AlreadyExistsError{msg: fmt.Sprintf("Slug %s already exists. Please choose another title.", slug)}
	}

	return nil
}

func (articlesService *ArticlesService) makeSlug(authorUsername string, title string) string {
	return slug.Make(fmt.Sprintf("%s %s", authorUsername, title))
}

func (articlesService *ArticlesService) makeTagName(tagName string) string {
	return slug.Make(tagName)
}
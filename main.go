package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/thedevsaddam/renderer"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

var rnd *renderer.Render
var db *mgo.Database

const (
	hostname       string = "localhost:27017"
	dbName         string = "demo_todo"
	collectionName string = "todo"
	port           string = ":9000"
)

// I need two different models - one is the JSON model which is used by the frontend (through HTTP requests) and the other one is BSON model, used to talk to MongoDB
type (
	todoModel struct {
		ID        bson.ObjectId `bson:"_id,omitempty"`
		Title     string        `bson:"title"`
		Completed bool          `bson:"completed"`
		CreatedAt time.Time     `bson:"createdAt"`
	}
	todo struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		Completed bool      `json:"completed"`
		CreatedAt time.Time `json:"created_at"`
	}
)

//Function init is used to connect to the mongodatabase and to set the consistency mode for the session
func init() {
	rnd = renderer.New()

	s, err := mgo.Dial(hostname)

	checkErr(err)
	s.SetMode(mgo.Monotonic, true)
	db = s.DB(dbName)
}

// TodoHandlers function defines a new router and the route paths for the API
func todoHandlers() http.Handler {
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Get("/", fetchTodos)
		r.Post("/", createTodo)
		r.Put("/{id}", updateTodo)
		r.Delete("/{id}", deleteTodo)
	})
	return r

}

//HomeHandler function renders the html template stored in the static folder
func homeHandler(w http.ResponseWriter, r *http.Request) {
	err := rnd.Template(w, http.StatusOK, []string{"static/home.tpl"}, nil)
	checkErr(err)

}

//FetchTodos function lists all the todo items
func fetchTodos(w http.ResponseWriter, r *http.Request) {
	todos := []todoModel{}
	if err := db.C(collectionName).Find(bson.M{}).All(&todos); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "failed to fetch todo",
			"error":   err,
		})
		return
	}
	todoList := []todo{}

	for _, t := range todos {
		todoList = append(todoList, todo{
			ID:        t.ID.Hex(),
			Title:     t.Title,
			Completed: t.Completed,
			CreatedAt: t.CreatedAt,
		})

	}
	rnd.JSON(w, http.StatusOK, renderer.M{
		"data": todoList,
	})
}

//CreateTodo function creates a todo item
func createTodo(w http.ResponseWriter, r *http.Request) {
	var t todo

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
	}

	if t.Title == "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "title is required",
		})
		return
	}

	tm := todoModel{
		ID:        bson.NewObjectId(),
		Title:     t.Title,
		Completed: false,
		CreatedAt: time.Now(),
	}

	if err := db.C(collectionName).Insert(&tm); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "failed to save todo",
			"error":   err,
		})
		return
	}
	rnd.JSON(w, http.StatusCreated, renderer.M{
		"messages": "todo created successfully",
		"ID":       tm.ID.Hex(),
	})
}

//UpdateTodo function update a todo item from the list based on the ID passed as URL parameter
func updateTodo(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))

	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "invalid ID",
		})
		return
	}

	var t todo

	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		rnd.JSON(w, http.StatusProcessing, err)
	}

	if t.Title != "" {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "title is required",
		})
		return
	}
	if err := db.C(collectionName).Update(
		bson.M{"_id": bson.ObjectIdHex(id)},
		bson.M{"title": t.Title, "completed": t.Completed},
	); err != nil {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "the id is invalid",
		})
		return
	}

}

// DeleteTodo deletes an item from the todolist, based on the ID passed as a parameter
func deleteTodo(w http.ResponseWriter, r *http.Request) {

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if !bson.IsObjectIdHex(id) {
		rnd.JSON(w, http.StatusBadRequest, renderer.M{
			"message": "invalid ID",
		})
		return
	}
	if err := db.C(collectionName).RemoveId(bson.ObjectIdHex(id)); err != nil {
		rnd.JSON(w, http.StatusProcessing, renderer.M{
			"message": "failed to delete todo",
			"error":   err,
		})
		return
	}
	rnd.JSON(w, http.StatusOK, renderer.M{
		"messages": "todo deleted successfully",
		"ID":       id,
	})
}

func checkErr(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {

	stopChan := make(chan os.Signal)
	signal.Notify(stopChan, os.Interrupt)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Get("/", homeHandler)
	r.Mount("/todo", todoHandlers())

	s := &http.Server{
		Addr:         port,
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		log.Println("Listening on port", port)
		if err := s.ListenAndServe(); err != nil {
			log.Printf("listen:%s\n", err)
		}
	}()
	<-stopChan
	log.Println("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	s.Shutdown(ctx)
	defer cancel()
	log.Println("server gracefully stopped")

}

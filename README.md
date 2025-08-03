# workspace

A modern todo application built with TheSkyscape DevTools, featuring real-time updates and beautiful UI components.

## Features

- ✅ **Task Management** - Create, edit, and organize todos with priorities
- 📅 **Due Dates** - Set and track deadlines for your tasks
- ⚡ **Real-time Updates** - HTMX-powered interface with no page refreshes
- 🎨 **Beautiful UI** - DaisyUI components with responsive design
- 🔐 **User Authentication** - Secure login and personal task lists
- 📊 **Statistics** - Track your productivity with todo stats

## Getting Started

1. **Install dependencies:**
   ```bash
   go mod tidy
   ```

2. **Run the application:**
   ```bash
   go run .
   ```

3. **Visit your app:**
   Open http://localhost:8080 in your browser

## Project Structure

```
workspace/
├── controllers/     # Request handlers
│   ├── home.go     # Home page controller
│   └── todos.go    # Todo CRUD operations
├── models/         # Data models
│   └── todo.go     # Todo model and repository
├── views/          # HTML templates
│   ├── layout.html # Main layout
│   ├── home/       # Home page views
│   └── todos/      # Todo-related views
├── main.go         # Application entry point
└── go.mod          # Dependencies
```

## Usage

### Creating Todos

1. **Sign up** or **sign in** to your account
2. Navigate to **My Todos**
3. Fill in the **Add New Todo** form:
   - **Title**: What needs to be done?
   - **Description**: Additional details (optional)
   - **Priority**: Low, Medium, or High
   - **Due Date**: When it should be completed (optional)

### Managing Todos

- **Complete**: Click the checkbox to mark as done
- **Uncomplete**: Click the checkbox again to mark as pending
- **Delete**: Click the trash icon to remove permanently

### Real-time Updates

The interface uses HTMX to provide real-time updates:
- New todos appear instantly when created
- Status changes update immediately
- No page refreshes required

## API Endpoints

The todo controller provides these endpoints:

- `GET /todos` - Display todo dashboard
- `POST /todos/create` - Create a new todo
- `POST /todos/complete` - Mark todo as completed
- `POST /todos/uncomplete` - Mark todo as pending
- `DELETE /todos/delete` - Delete a todo

## Customization

### Adding New Fields

To add new fields to todos:

1. **Update the model:**
   ```go
   // models/todo.go
   type Todo struct {
       database.Model
       // ... existing fields
       Category string `json:"category"`
   }
   ```

2. **Update the form:**
   ```html
   <!-- views/todos/index.html -->
   <input type="text" name="category" class="input input-bordered">
   ```

3. **Update the controller:**
   ```go
   // controllers/todos.go
   todo.Category = t.Request.FormValue("category")
   ```

### Styling

The application uses DaisyUI themes. Change the theme using environment variables:

```bash
export THEME=dark          # Dark theme
export THEME=light         # Light theme
export THEME=corporate     # Corporate theme (default)
export THEME=retro         # Retro theme
```

Available themes: https://daisyui.com/docs/themes/

## Deployment

Deploy to DigitalOcean using the launch-app tool:

```bash
launch-app workspace --provider=digitalocean
```

## Development

### Hot Reload

Install Air for automatic reloading:

```bash
go install github.com/cosmtrek/air@latest
air
```

### Adding Authentication Features

The app includes built-in authentication. To customize:

- **Signup/Signin views**: Modify authentication templates
- **User profiles**: Extend the User model
- **Permissions**: Add role-based access control

### Database

Uses SQLite with automatic migrations. The database file is created automatically at:
- Development: `./data/workspace.db`
- Production: `~/.theskyscape/workspace.db`

## Documentation

- [TheSkyscape DevTools Documentation](https://github.com/The-Skyscape/devtools)
- [DaisyUI Components](https://daisyui.com/components/)
- [HTMX Documentation](https://htmx.org/docs/)
- [Go Templates](https://pkg.go.dev/text/template)

## Contributing

1. Fork the project
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## License

This project is licensed under the MIT License.
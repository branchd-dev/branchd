# Branchd Database Isolation Demo

This is a Next.js demo application that demonstrates database branch isolation for Branchd. It allows you to connect to multiple PostgreSQL databases simultaneously and perform CRUD operations on a `users` table to show that each branch is completely isolated from the others.

## Features

- **Three Independent Database Connections**: Connect to three different PostgreSQL instances simultaneously
- **Real-time CRUD Operations**: Create, Read, Update, and Delete users in each database
- **Error Handling**: Gracefully handles connection errors, including deleted branches
- **Refresh All**: Quickly refresh data from all connected databases
- **Isolated Operations**: Demonstrates that operations on one branch don't affect others

## Getting Started

### Prerequisites

- Node.js 18+ and npm
- PostgreSQL database instances (can be Branchd branches)

### Installation

1. Navigate to the demo directory:
   ```bash
   cd demo
   ```

2. Install dependencies (already done if you just set up the project):
   ```bash
   npm install
   ```

3. Run the development server:
   ```bash
   npm run dev
   ```

4. Open [http://localhost:3000](http://localhost:3000) in your browser

## How to Use

### Basic Usage

1. **Enter Connection Strings**: Paste PostgreSQL connection strings into any of the three connection string fields. For example:
   ```
   postgresql://user:password@localhost:5432/database1
   postgresql://user:password@localhost:5432/database2
   postgresql://user:password@localhost:5432/database3
   ```

2. **Connect**: Click the "Connect" button next to each connection string to connect to the database

3. **View Users**: The app will automatically create a `users` table if it doesn't exist and display all users

4. **Add Users**: Use the "Add New User" form at the bottom of each section to insert new users

5. **Edit Users**: Click the "Edit" button next to any user to modify their name or email

6. **Delete Users**: Click the "Delete" button to remove a user (with confirmation)

7. **Refresh All**: Use the "Refresh All" button at the top to reload data from all connected databases

### Demonstrating Branch Isolation

To demonstrate that branches are isolated:

1. Connect to three different Branchd branches (or three different databases)
2. Add different users to each branch
3. Observe that each branch maintains its own independent data
4. Perform operations (add/edit/delete) on one branch and use "Refresh All" to confirm other branches are unaffected

### Demonstrating Branch Deletion

To demonstrate what happens when a branch is deleted:

1. Connect to a Branchd branch and add some users
2. Delete the branch using the Branchd CLI or API
3. Click "Refresh All" or try to perform any operation
4. The app will display an error message showing that the database is no longer accessible

## API Routes

The demo app includes the following API endpoints:

- `GET /api/db?connectionString=<conn_string>` - Fetch all users from a database
- `POST /api/db` - Insert a new user (body: `{ connectionString, name, email }`)
- `PUT /api/db` - Update a user (body: `{ connectionString, id, name, email }`)
- `DELETE /api/db?connectionString=<conn_string>&id=<user_id>` - Delete a user

## Database Schema

The app automatically creates the following table structure:

```sql
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  name VARCHAR(255) NOT NULL,
  email VARCHAR(255) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Technology Stack

- **Next.js 16** - React framework with App Router
- **TypeScript** - Type safety
- **Tailwind CSS** - Styling
- **pg** - PostgreSQL client for Node.js
- **React** - UI components

## Development

```bash
# Run development server
npm run dev

# Build for production
npm run build

# Start production server
npm start
```

## Notes

- The app creates a new connection pool for each request and closes it after the operation completes
- Connection timeout is set to 5 seconds to quickly detect unavailable databases
- The `users` table is automatically created if it doesn't exist on first connection
- All database operations are performed server-side through Next.js API routes for security

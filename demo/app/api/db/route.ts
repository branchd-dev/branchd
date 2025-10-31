import { NextRequest, NextResponse } from 'next/server';
import { Pool } from 'pg';

// Helper function to create a connection pool
function createPool(connectionString: string) {
  return new Pool({
    connectionString,
    connectionTimeoutMillis: 5000,
    max: 1,
    ssl: {
      rejectUnauthorized: false, // Accept self-signed certificates
    },
  });
}

// GET - Fetch users from the database
export async function GET(request: NextRequest) {
  const searchParams = request.nextUrl.searchParams;
  const connectionString = searchParams.get('connectionString');

  if (!connectionString) {
    return NextResponse.json(
      { error: 'Connection string is required' },
      { status: 400 }
    );
  }

  const pool = createPool(connectionString);

  try {
    const client = await pool.connect();
    try {
      // Fetch all users
      const result = await client.query(
        'SELECT id, name, email, phone, address, ssn FROM users ORDER BY id'
      );

      return NextResponse.json({
        success: true,
        users: result.rows,
      });
    } finally {
      client.release();
    }
  } catch (error: any) {
    console.error('Database error:', error);
    return NextResponse.json(
      {
        success: false,
        error: error.message || 'Failed to connect to database',
      },
      { status: 500 }
    );
  } finally {
    await pool.end();
  }
}

// POST - Insert a new user
export async function POST(request: NextRequest) {
  const body = await request.json();
  const { connectionString, name, email, phone, address, ssn } = body;

  if (!connectionString || !name || !email) {
    return NextResponse.json(
      { error: 'Connection string, name, and email are required' },
      { status: 400 }
    );
  }

  const pool = createPool(connectionString);

  try {
    const client = await pool.connect();
    try {
      // Insert new user
      const result = await client.query(
        'INSERT INTO users (name, email, phone, address, ssn) VALUES ($1, $2, $3, $4, $5) RETURNING id, name, email, phone, address, ssn',
        [name, email, phone, address, ssn]
      );

      return NextResponse.json({
        success: true,
        user: result.rows[0],
      });
    } finally {
      client.release();
    }
  } catch (error: any) {
    console.error('Database error:', error);
    return NextResponse.json(
      {
        success: false,
        error: error.message || 'Failed to insert user',
      },
      { status: 500 }
    );
  } finally {
    await pool.end();
  }
}

// PUT - Update a user
export async function PUT(request: NextRequest) {
  const body = await request.json();
  const { connectionString, id, name, email, phone, address, ssn } = body;

  if (!connectionString || !id || !name || !email) {
    return NextResponse.json(
      { error: 'Connection string, id, name, and email are required' },
      { status: 400 }
    );
  }

  const pool = createPool(connectionString);

  try {
    const client = await pool.connect();
    try {
      const result = await client.query(
        'UPDATE users SET name = $1, email = $2, phone = $3, address = $4, ssn = $5 WHERE id = $6 RETURNING id, name, email, phone, address, ssn',
        [name, email, phone, address, ssn, id]
      );

      if (result.rows.length === 0) {
        return NextResponse.json(
          { success: false, error: 'User not found' },
          { status: 404 }
        );
      }

      return NextResponse.json({
        success: true,
        user: result.rows[0],
      });
    } finally {
      client.release();
    }
  } catch (error: any) {
    console.error('Database error:', error);
    return NextResponse.json(
      {
        success: false,
        error: error.message || 'Failed to update user',
      },
      { status: 500 }
    );
  } finally {
    await pool.end();
  }
}

// DELETE - Delete a user
export async function DELETE(request: NextRequest) {
  const searchParams = request.nextUrl.searchParams;
  const connectionString = searchParams.get('connectionString');
  const id = searchParams.get('id');

  if (!connectionString || !id) {
    return NextResponse.json(
      { error: 'Connection string and id are required' },
      { status: 400 }
    );
  }

  const pool = createPool(connectionString);

  try {
    const client = await pool.connect();
    try {
      const result = await client.query(
        'DELETE FROM users WHERE id = $1 RETURNING id',
        [id]
      );

      if (result.rows.length === 0) {
        return NextResponse.json(
          { success: false, error: 'User not found' },
          { status: 404 }
        );
      }

      return NextResponse.json({
        success: true,
      });
    } finally {
      client.release();
    }
  } catch (error: any) {
    console.error('Database error:', error);
    return NextResponse.json(
      {
        success: false,
        error: error.message || 'Failed to delete user',
      },
      { status: 500 }
    );
  } finally {
    await pool.end();
  }
}

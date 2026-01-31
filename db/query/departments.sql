-- name: ListDepartments :many
SELECT id, name, parent_id, created_at
FROM departments
ORDER BY id DESC;

-- name: CreateDepartment :exec
INSERT INTO departments (name, parent_id)
VALUES (?, ?);

-- name: UpdateDepartment :exec
UPDATE departments
SET name = ?, parent_id = ?
WHERE id = ?;

-- name: DeleteDepartment :exec
DELETE FROM departments
WHERE id = ?;

-- name: CountEmployeesByDepartment :one
SELECT COUNT(1)
FROM employees
WHERE department_id = ?;

-- Insert a test account
insert into accounts (id, name) values ('a_409f852bf39791ccc2496d23f18c63ac', 'Account 1');

-- Insert an admin user
insert into users (id, account_id, name, email, confirmed, active, role)
values ('u_f4958e9cd27a553b08092c790ea44fbb', 'a_409f852bf39791ccc2496d23f18c63ac', 'Admin', 'admin@example.com', 1, 1, 'admin');

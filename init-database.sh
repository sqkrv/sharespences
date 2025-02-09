#!/bin/bash

# Prompt for the database password securely
read -sp 'Enter the database password: ' DB_PASSWORD
echo

# Export the password to the PGPASSWORD environment variable
export PGPASSWORD=$DB_PASSWORD

# Execute the SQL commands
echo "Copying data into bank table..."
cat sql_data/banks.csv | psql-17 -U sharespences -h squidass.com -p 9432 -d sharespences -c "copy bank (id, name, logo_filename) from stdin with (format csv, header true, delimiter ';')"

echo "Copying data into mcc_code table..."
cat sql_data/mcc_codes.csv | psql-17 -U sharespences -h squidass.com -p 9432 -d sharespences -c "copy mcc_code (code, name, description) from stdin with (format csv, header true, delimiter ';')"

echo "Copying data into category table..."
cat sql_data/categories.csv | psql-17 -U sharespences -h squidass.com -p 9432 -d sharespences -c "copy category (id, bank_id, name, description, icon_filename, mcc_additional_description, og_id) from stdin with (format csv, header true, delimiter ';')"

echo "Copying data into category_mcc table..."
cat sql_data/category_mcc.csv | psql-17 -U sharespences -h squidass.com -p 9432 -d sharespences -c "copy category_mcc (category_id, mcc_code) from stdin with (format csv, header true, delimiter ';')"

echo "Copying data into bank_mcc table..."
cat sql_data/bank_mcc.csv | psql-17 -U sharespences -h squidass.com -p 9432 -d sharespences -c "copy bank_mcc (bank_id, mcc_code, footnote) from stdin with (format csv, header true, delimiter ';')"

# Unset the PGPASSWORD environment variable
unset PGPASSWORD
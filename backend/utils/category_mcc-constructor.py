import csv
import logging
import re
from pathlib import Path

from pydantic import BaseModel

logger = logging.getLogger(__name__)

def parse_mcc(category_id, mcc_field: str) -> tuple[list[int], str | None]:
    mcc_codes = []
    additional_description = None

    matches = re.findall(r"(\d{4})\s*-\s*(\d{4})|(\d{4})", mcc_field)
    for match in matches:
        if match[0]:
            mcc_codes.extend(range(int(match[0]), int(match[1]) + 1))
        elif match[2]:
            mcc_codes.append(int(match[2]))

    if re.search(r"[А-я]|[A-z]", mcc_field):
        additional_description = mcc_field

    if len(mcc_codes) != len(set(mcc_codes)):
        logger.warning(f"Duplicate MCC codes in category ID: {category_id}. Removing duplicates.")
        mcc_codes = list(set(mcc_codes))

    return mcc_codes, additional_description

def process_categories(
        raw_categories_csv_file: Path,
        mcc_codes_csv_file: Path,
        categories_mcc_csv_file: Path,
        categories_csv_file: Path
):
    with (raw_categories_csv_file.open() as infile,
          mcc_codes_csv_file.open() as mcc_codes_infile,
          categories_mcc_csv_file.open('w', newline='', encoding='utf-8') as categories_mcc_outfile,
          categories_csv_file.open('w', newline='', encoding='utf-8') as categories_csv_file):

        class CategoryRow(BaseModel):
            id: int
            bank_id: int
            name: str
            description: str
            icon_filename: str
            mcc_additional_description: str
            og_id: str

        class CategoryMCCRow(BaseModel):
            category_id: int
            mcc_code: int

        reader = csv.DictReader(infile, delimiter=';')
        mcc_codes_reader = csv.DictReader(mcc_codes_infile, delimiter=';')
        categories_writer = csv.DictWriter(categories_csv_file, CategoryRow.model_fields.keys(), delimiter=';')
        categories_mcc_writer = csv.DictWriter(categories_mcc_outfile, CategoryMCCRow.model_fields.keys(), delimiter=';')

        existing_mcc_codes = {int(row['code']) for row in mcc_codes_reader}

        categories_writer.writeheader()
        categories_mcc_writer.writeheader()

        # categories_mcc_writer.writerow(['category_id', 'mcc_code'])
        # categories_writer.writerow(['id', 'bank_id', 'name', 'description', 'icon_filename', 'mcc_additional_description', 'og_id'])

        for row in reader:
            category_id = row['id']
            mcc_field = row['mcc']

            mcc_codes, additional_description = parse_mcc(category_id, mcc_field)
            for mcc_code in mcc_codes:
                if mcc_code not in existing_mcc_codes:
                    logger.warning(f"Unknown MCC code: {mcc_code}. Skipping.")
                    continue
                categories_mcc_writer.writerow(CategoryMCCRow(category_id=category_id, mcc_code=mcc_code).model_dump())

            # row_dict = {key: value for key, value in row.items() if key != 'mcc'}
            # row_dict['mcc_additional_description'] = additional_description or ''
            categories_writer.writerow(CategoryRow(**row | {'mcc_additional_description': additional_description or ''}).model_dump())

if __name__ == '__main__':
    BASE_PATH = Path("/Users/sqkrv/PycharmProjects/sharespences/sql_data")
    process_categories(
        Path(BASE_PATH / "raw_categories.csv"),
        Path(BASE_PATH / "mcc_codes.csv"),
        Path(BASE_PATH / "category_mcc.csv"),
        Path(BASE_PATH / "categories.csv")
    )
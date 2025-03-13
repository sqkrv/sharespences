import csv
import datetime
import logging
import os
import sys
from enum import Enum
from pathlib import Path
from uuid import UUID

import httpx
from bs4 import BeautifulSoup
from pydantic import BaseModel

logger = logging.getLogger(__name__)
logging.basicConfig(format='%(message)s')
logger.setLevel(logging.INFO)


class PointTypeEnum(str, Enum):
    offline = 'offline'
    online = 'online'
    app = 'app'
    other = 'other'


class Point(BaseModel):
    id: UUID
    title: str
    merchant_title: str | None
    mcc: int
    type: PointTypeEnum | None
    address: str | None
    confirmations: int
    created_at: datetime.date
    actual_at: datetime.date | None


BASE_URL = "https://mcc-codes.ru"


class PointsOfSaleParser:
    def __init__(self, mcc_codes_csv_file: Path):
        self.client = httpx.Client()
        self.mcc_codes_csv_file = mcc_codes_csv_file

    def get_mcc_codes(self, mcc_codes_csv_file: Path, *, sort: bool = True) -> list[int]:
        """Extracts all MCC codes from the website."""
        with mcc_codes_csv_file.open() as mcc_codes_file:
            mcc_codes_reader = csv.DictReader(mcc_codes_file, delimiter=';')
            mcc_codes = [int(row['code']) for row in mcc_codes_reader]
        logger.info(f"Found {len(mcc_codes)} MCC codes")
        if sort:
            mcc_codes.sort()
        return mcc_codes

    @staticmethod
    def _format_merchant_title(merchant_title: str) -> str:
        merchant_title = merchant_title.strip()
        if merchant_title.startswith('['):
            merchant_title = merchant_title[1:]
        if merchant_title.endswith(']'):
            merchant_title = merchant_title[:-1]
        return merchant_title

    def get_points_page(
            self,
            *,
            mcc_code: int,
            page: int,
            sort_by: str = 'date',
            sort_dir: str = 'asc'
    ):
        params = {
            'extended': '1',
            'm': f"{mcc_code:04}",
            'sortBy': sort_by,
            'sortDir': sort_dir,
            'page': page,
        }

        response = self.client.get(f"{BASE_URL}/search", params=params)
        return response

    def get_points_for_mcc(
            self,
            mcc_code: int,
    ) -> tuple[list[Point], bool]:
        """Extracts all points of sale for a given MCC code.

        :param mcc_code: The MCC code to extract points for.
        :return: A list of points and a boolean indicating if an error occurred
        """
        points = []
        page = 1
        while True:
            try:
                logger.info(f"[MCC: {mcc_code:04}] [Page: {page}] — Fetching page")
                response = self.get_points_page(mcc_code=mcc_code, page=page)
                soup = BeautifulSoup(response.text, "html.parser")

                rows = soup.select("#points-search-table tbody tr")
                if not rows:
                    break  # No more data

                for row in rows:
                    columns = row.find_all("td")
                    if len(columns) < 4:
                        continue

                    id = UUID(row.get("data-s"))
                    mcc_code = int(columns[0].text.strip())
                    title = columns[1].find("b").text.strip()
                    merchant_title = columns[1].find("span", class_=lambda _: "text-uppercase text-muted" in _)
                    merchant_title = self._format_merchant_title(merchant_title.text) if merchant_title else None

                    class_list = columns[2].span.get('class')
                    if 'oi-map-marker' in class_list:
                        point_type = PointTypeEnum.offline
                    elif 'oi-globe' in class_list:
                        point_type = PointTypeEnum.online
                    elif 'oi-phone' in class_list:
                        point_type = PointTypeEnum.app
                    elif 'oi-pencil' in class_list:
                        point_type = PointTypeEnum.other
                    else:
                        point_type = None

                    address = columns[2].text.strip()

                    confirmation_badge = columns[3].find("span", class_="badge bg-success p-1")
                    confirmations = int(confirmation_badge.text.strip()) if confirmation_badge else 0

                    date_element = columns[3].find("b")
                    if date_element and "title" in date_element.attrs:
                        date_tooltip = date_element["title"].split("<br/>")
                        created_at = date_tooltip[-2].split(": ")[-1]
                        actual_at = date_tooltip[-1].split(": ")[-1]
                    else:
                        # If no confirmation, assume the only available date is the creation date
                        created_at = columns[3].text.strip()
                        actual_at = None

                    created_at = datetime.datetime.strptime(created_at, "%d.%m.%Y").date()
                    if actual_at is not None:
                        actual_at = datetime.datetime.strptime(actual_at, "%d.%m.%Y").date()

                    points.append(Point(
                        id=id,
                        title=title,
                        merchant_title=merchant_title,
                        mcc=mcc_code,
                        type=point_type,
                        address=address,
                        confirmations=confirmations,
                        created_at=created_at,
                        actual_at=actual_at
                    ))

                page += 1
            except Exception as e:
                logger.error(f"[MCC: {mcc_code:04}] [Page: {page}] — Error: {e}\nwhile parsing: {row}")
                return points, True

        logger.info(f"[MCC: {mcc_code:04}] — Found {len(points)} points")

        return points, False

    def parse_all_points(self) -> list[Point]:
        """Extracts all points of sale for all MCC codes."""
        all_points = []
        mcc_codes = self.get_mcc_codes(self.mcc_codes_csv_file)
        for mcc_code in mcc_codes:
            logger.info(f"[MCC: {mcc_code:04}] — Starting parsing")
            points, errored = self.get_points_for_mcc(mcc_code)
            all_points.extend(points)
            if errored:
                break
        return all_points


if __name__ == "__main__":
    mcc_codes_csv_file = sys.argv[1] if len(sys.argv) > 1 else os.getenv("MCC_CODES_CSV_FILE")
    points_of_sale_csv_file = sys.argv[2] if len(sys.argv) > 2 else os.getenv("POINTS_OF_SALE_CSV_FILE")
    parser = PointsOfSaleParser(Path(mcc_codes_csv_file))
    logger.info("Starting execution")
    all_points = parser.parse_all_points()
    logger.info(f"Total points: {len(all_points)}")
    logger.info("Writing to CSV")
    with Path(points_of_sale_csv_file).open('w', newline='', encoding='utf-8') as points_outfile:
        points_writer = csv.DictWriter(points_outfile, Point.model_fields.keys(), delimiter=';')
        points_writer.writeheader()
        for point in all_points:
            points_writer.writerow(point.model_dump())
    logger.info("Done writing to CSV")

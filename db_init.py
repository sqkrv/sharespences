from sqlalchemy import insert

from backend.db_models.db_base_class import Base
import backend.db_models.models
from backend.db_models.models import BankDB
from backend.db_models.db_session import SessionLocal, sync_engine

if __name__ == '__main__':
    with SessionLocal() as session:

        print(Base.metadata.tables.keys())
        input("Continue?")
        Base.metadata.drop_all(bind=sync_engine)
        Base.metadata.create_all(bind=sync_engine)
        print(Base.metadata.tables.keys())

        banks = [
                {"name": "Альфа-Банк"},
                {"name": "Сбербанк"},
                {"name": "Тинькофф"},
                {"name": "ВТБ"},
                {"name": "Ozon банк"},
                {"name": "Яндекс Pay"},
        ]
        session.execute(insert(BankDB), banks)

        session.commit()

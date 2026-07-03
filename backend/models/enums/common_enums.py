from enum import Enum


class Period(str, Enum):
    day = "day"
    week = "week"
    month = "month"
    year = "year"

class Direction(str, Enum):
    expense = "expense"
    income = "income"

class Status(str, Enum):
    success = "success"
    hold = "hold"

class PaymentSystem(str, Enum):
    visa = "visa"
    mastercard = "mastercard"
    mir = "mir"
    unionpay = "unionpay"
    american_express = "american_express"

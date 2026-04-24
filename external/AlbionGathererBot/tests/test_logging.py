import os
import pytest
import logging
from unittest import mock
import sys
from io import StringIO
import sys
import importlib
sys.path.append(os.path.join(os.path.dirname(__file__), '..'))
import app_logging

@pytest.fixture
def clean_env():
    # Remove APP_LOG_LEVEL from env to have a clean state
    if "APP_LOG_LEVEL" in os.environ:
        del os.environ["APP_LOG_LEVEL"]
    yield

def test_logger_creates_directory_and_file(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)

    # We must ensure app_logging doesn't return cached root level handlers and uses a fresh logger for these tests.
    importlib.reload(app_logging)
    logger = app_logging.get_logger("test_dir_logger_2")

    assert os.path.exists("logs")
    assert os.path.exists(os.path.join("logs", "app.log"))
    assert logger.level == logging.INFO

def test_logger_level_debug(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("APP_LOG_LEVEL", "DEBUG")

    importlib.reload(app_logging)
    logger = app_logging.get_logger("test_debug_logger_2")

    assert logger.level == logging.DEBUG

def test_logger_level_invalid_fallback(tmp_path, monkeypatch):
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("APP_LOG_LEVEL", "INVALID")

    importlib.reload(app_logging)
    logger = app_logging.get_logger("test_invalid_logger_2")

    assert logger.level == logging.INFO

def test_redact_sensitive():
    assert app_logging.redact_sensitive("my_secret_token_123") == "***REDACTED***"
    assert app_logging.redact_sensitive("my_password_123") == "***REDACTED***"
    assert app_logging.redact_sensitive("hello world") == "hello world"
    assert app_logging.redact_sensitive(123) == 123

def test_safe_path():
    assert app_logging.safe_path("/home/user/myproject/file.txt") == "file.txt"
    assert app_logging.safe_path("C:\\Users\\user\\Desktop\\file.txt") == "file.txt"

def test_summarize_event():
    assert app_logging.summarize_event("mouse", "click", x=100, y=200) == "action=click, type=mouse, x=100, y=200"

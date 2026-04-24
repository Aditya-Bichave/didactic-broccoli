import logging
import logging.handlers
import os
import sys

def get_logger(name="app"):
    logger = logging.getLogger(name)
    logger.handlers.clear() # Clear all handlers to be clean

    log_level_str = os.environ.get("APP_LOG_LEVEL", "INFO").upper()
    try:
        log_level = getattr(logging, log_level_str)
        if not isinstance(log_level, int):
            log_level = logging.INFO
    except AttributeError:
        log_level = logging.INFO

    logger.setLevel(log_level)

    log_dir = "logs"
    os.makedirs(log_dir, exist_ok=True)
    log_file = os.path.join(log_dir, "app.log")

    formatter = logging.Formatter("%(asctime)s | %(levelname)s | %(name)s | %(threadName)s | %(message)s")

    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setFormatter(formatter)
    logger.addHandler(console_handler)

    file_handler = logging.handlers.RotatingFileHandler(
        log_file, maxBytes=5 * 1024 * 1024, backupCount=5, encoding="utf-8"
    )
    file_handler.setFormatter(formatter)
    logger.addHandler(file_handler)

    return logger

logger = get_logger()

def redact_sensitive(value):
    if not isinstance(value, str):
        return value
    return "***REDACTED***" if "token" in value.lower() or "password" in value.lower() else value

def safe_path(path):
    path = str(path).replace("\\", "/")
    return os.path.basename(path)

def summarize_event(event_type, action, **kwargs):
    summary = f"action={action}, type={event_type}"
    for k, v in kwargs.items():
        summary += f", {k}={v}"
    return summary

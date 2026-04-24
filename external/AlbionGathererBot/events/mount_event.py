import sys
import os
sys.path.append(os.path.join(os.path.dirname(__file__), '..'))
from app_logging import logger
class  MountEvent:
    def __init__(self, parameters):
        self.mounted = True if (2 in parameters) else False
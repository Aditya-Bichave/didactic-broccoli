import sys
import os
sys.path.append(os.path.join(os.path.dirname(__file__), '..'))
from app_logging import logger
class LeaveEvent:
    def __init__(self, parameters):
        self.objectId = 0

        if 0 in parameters:
            self.objectId = parameters[0]
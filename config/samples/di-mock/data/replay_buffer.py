from queue import Queue

class ReplayBuffer:
    def __init__(self, cfg: dict):
        self._meta_buffer = []

    def push_data(self, data) -> None:
        self._meta_buffer.append(data)

    def sample(self, batch_size: int):
        data = None
        if len(self._meta_buffer) >= batch_size:
            data = self._meta_buffer[:batch_size]
            self._meta_buffer = self._meta_buffer[batch_size:]
        return data

    def run(self):
        pass

    def close(self):
        pass
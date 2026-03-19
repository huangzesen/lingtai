"""Re-export kernel mail services for backward compatibility."""
from stoai_kernel.services.mail import MailService, TCPMailService

__all__ = ["MailService", "TCPMailService"]

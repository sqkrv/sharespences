import ContactList from "./Contacts";

export const ContactListPage = () => {
  const contacts = [
    { id: "1", fullName: "Иван Иванов" },
    { id: "2", fullName: "Петр Петров" },
    { id: "3", fullName: "Сергей Сергеев" },
    { id: "4", fullName: "Алексей Алексеев" },
    { id: "5", fullName: "Алексей Алексеев" },
    { id: "6", fullName: "Алексей Алексеев" },
    { id: "7", fullName: "Алексей Алексеев" },
  ];

  return (
    <div>
      <ContactList contacts={contacts} />
    </div>
  );
};

export default ContactListPage;
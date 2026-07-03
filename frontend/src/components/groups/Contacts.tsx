import React from "react";

type Contact = {
  id: string;
  fullName: string;
};

type ContactListProps = {
  contacts: Contact[];
};

const ContactList: React.FC<ContactListProps> = ({ contacts }) => {
  const formatName = (name: string) => {
    const [firstName, lastName] = name.split(' ');
    return `${firstName} ${lastName?.[0] || ''}.`;
  };

  return (
    <div className="relative touch-pan-x"> {}
      <div 
        className="flex overflow-x-auto whitespace-nowrap py-2 hide-scrollbar"
        style={{ WebkitOverflowScrolling: 'touch' }}
      >
        {contacts.map((contact) => (
          <div 
            key={contact.id} 
            className="inline-flex flex-col items-center mx-2 flex-shrink-0 w-16"
          >
            <div className="w-16 h-16 rounded-full bg-gray-200 flex items-center justify-center mb-1">
              <span className="text-h4 text-gray-700">
                {contact.fullName.split(' ').map(n => n[0]).join('')}
              </span>
            </div>
            <span className="text-text text-gray-600 text-center">
              {formatName(contact.fullName)}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
};

export default ContactList;
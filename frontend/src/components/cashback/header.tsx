export default function Header() {
    return (
      <header className="pt-16 px-2 flex justify-between items-center font-sans">
        {/* Profile Section */}
        <div className="flex items-center gap-2">
          <img src="/user-avatar.png" alt="User Avatar" className="w-8 h-8 rounded-full" />
          <h4 className="text-font font-semibold text-h4">Nickname</h4>
        </div>
        
        {/* Icons Section */}
        <div className="flex items-center gap-2 font-sans">
          <img src="/icons/search-icon.svg" alt="Search" className="w-8 h-8" />
          <img src="/settings-icon.svg" alt="Settings" className="w-8 h-8" />
        </div>
      </header>
    );
}
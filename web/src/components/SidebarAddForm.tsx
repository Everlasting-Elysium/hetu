import { useState } from "react";
import { IconPlus } from "./icons";
import styles from "./Sidebar.module.css";

// Inline "add" form toggled per sidebar section (folders/tags/collections). Its
// own module so both Sidebar and SidebarCollections reuse the exact input+submit
// affordance without a circular import.
export function AddForm({
  placeholder,
  onSubmit,
}: {
  placeholder: string;
  onSubmit: (v: string) => void;
}) {
  const [v, setV] = useState("");
  return (
    <form
      className={styles.form}
      onSubmit={(e) => {
        e.preventDefault();
        if (v.trim()) {
          onSubmit(v.trim());
          setV("");
        }
      }}
    >
      <input
        autoFocus
        className="input"
        placeholder={placeholder}
        value={v}
        onChange={(e) => setV(e.target.value)}
      />
      <button type="submit" className="btn btn-primary btn-icon" title="创建">
        <IconPlus width={14} height={14} />
      </button>
    </form>
  );
}

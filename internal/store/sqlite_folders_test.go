package store_test

import (
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func mkFolder(t *testing.T, owner domain.OwnerID, id, name, path string) domain.Folder {
	t.Helper()
	fid, err := domain.NewFolderID(id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Folder{ID: fid, Owner: owner, Name: name, Path: path}
}

func TestSQLite_FolderCRUD(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	photos := mkFolder(t, owner, "f1", "Photos", "/Photos")
	videos := mkFolder(t, owner, "f2", "Videos", "/Videos")
	if err := st.CreateFolder(ctx, photos); err != nil {
		t.Fatalf("create photos: %v", err)
	}
	if err := st.CreateFolder(ctx, videos); err != nil {
		t.Fatalf("create videos: %v", err)
	}

	folders, err := st.ListFolders(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 {
		t.Fatalf("folders = %d, want 2", len(folders))
	}
	// Ordered by path.
	if folders[0].Path != "/Photos" || folders[1].Path != "/Videos" {
		t.Fatalf("order = %q,%q, want /Photos,/Videos", folders[0].Path, folders[1].Path)
	}

	if err := st.DeleteFolder(ctx, owner, photos.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	folders, err = st.ListFolders(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || folders[0].Name != "Videos" {
		t.Fatalf("after delete = %+v, want [Videos]", folders)
	}
}

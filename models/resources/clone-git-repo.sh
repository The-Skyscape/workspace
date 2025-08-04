git config --global --add safe.directory /home/coder/%[3]s
git config --global init.defaultBranch master

git config --global user.name "%[4]s"
git config --global user.email "%[5]s"

git clone http://%[1]s:%[2]s@localhost/repo/%[3]s /home/coder/%[3]s
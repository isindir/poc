package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/google/go-github/v50/github"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// Max retries for failed pushes
const maxRetries = 5

// PushDelay is used for exponential backoff
const pushDelay = 2 * time.Second

func main() {
	// GitHub repository details
	owner := "isindir"
	repo := "poc"
	branch := "master"

	// OAuth Token from the environment
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("GITHUB_TOKEN")},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// WaitGroup to wait for all pushes to complete
	var wg sync.WaitGroup

	// Loop to launch 100 concurrent pushes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			fileID := uuid.New().String()
			filePath := fmt.Sprintf("unique-file-%s.txt", fileID)
			content := fmt.Sprintf("This is unique content for file %d with UUID %s", i, fileID)

			commitMessage := fmt.Sprintf("Adding file %d with UUID %s", i, fileID)

			// Attempt to push with retries
			pushWithRetry(ctx, client, owner, repo, branch, filePath, content, commitMessage)
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	fmt.Println("All pushes to main branch are done!")
}

// Function to push with retry logic
func pushWithRetry(ctx context.Context, client *github.Client, owner, repo, branch, filePath, content, commitMessage string) {
	var err error
	for retries := 0; retries < maxRetries; retries++ {
		if retries > 0 {
			fmt.Printf("Retrying push for file %s (attempt %d)...\n", filePath, retries+1)
			time.Sleep(pushDelay * time.Duration(rand.Intn(retries+1)))
		}

		// Perform the push
		err = pushToMainBranch(ctx, client, owner, repo, branch, filePath, content, commitMessage)
		if err == nil {
			fmt.Printf("Successfully pushed file %s to the main branch!\n", filePath)
			return
		}

		log.Printf("Error pushing file %s: %v", filePath, err)
	}

	log.Fatalf("Failed to push file %s after %d attempts", filePath, maxRetries)
}

// Function to push a single file to the main branch
func pushToMainBranch(ctx context.Context, client *github.Client, owner, repo, branch, filePath, content, commitMessage string) error {
	// Get the repository reference (head of the branch)
	ref, _, err := client.Git.GetRef(ctx, owner, repo, "refs/heads/"+branch)
	if err != nil {
		return fmt.Errorf("error getting repository reference: %v", err)
	}

	// Get the latest commit
	commit, _, err := client.Git.GetCommit(ctx, owner, repo, *ref.Object.SHA)
	if err != nil {
		return fmt.Errorf("error getting latest commit: %v", err)
	}

	// Create a new tree with the unique file
	treeEntries := []*github.TreeEntry{
		{
			Path:    github.String(filePath),
			Type:    github.String("blob"),
			Content: github.String(content),
			Mode:    github.String("100644"),
		},
	}
	tree, _, err := client.Git.CreateTree(ctx, owner, repo, *commit.Tree.SHA, treeEntries)
	if err != nil {
		return fmt.Errorf("error creating tree: %v", err)
	}

	// Create a new commit
	newCommit := &github.Commit{
		Message: github.String(commitMessage),
		Tree:    tree,
		Parents: []*github.Commit{commit},
	}
	commitResponse, _, err := client.Git.CreateCommit(ctx, owner, repo, newCommit)
	if err != nil {
		return fmt.Errorf("error creating commit: %v", err)
	}

	// Update the reference to point to the new commit
	ref.Object.SHA = commitResponse.SHA
	_, _, err = client.Git.UpdateRef(ctx, owner, repo, ref, false)
	if err != nil {
		return fmt.Errorf("error updating ref: %v", err)
	}

	return nil
}

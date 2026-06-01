#include <iostream>
#include <string>
#include <vector>
#include <iomanip>

using namespace std;

// --- Book Class ---
class Book {
private:
    int id;
    string title;
    string author;
    bool isIssued;

public:
    Book(int id, string t, string a) {
        this->id = id;
        title = t;
        author = a;
        isIssued = false;
    }

    int getId() const { return id; }
    string getTitle() const { return title; }
    string getAuthor() const { return author; }
    bool getStatus() const { return isIssued; }

    void issueBook() { isIssued = true; }
    void returnBook() { isIssued = false; }

    void display() const {
        cout << left << setw(5) << id << setw(20) << title 
             << setw(20) << author << setw(15) 
             << (isIssued ? "Issued" : "Available") << endl;
    }
} // ERROR 1: Something is missing here

// --- Library Class ---
class Library {
private:
    vector<Book> books;

public:
    void addBook(int id, string title, string author) {
        books.push_back(Book(id, title, author));
        cout << "Book added successfully!\n";
    }

    void displayAll() const {
        if (books.empty()) {
            cout << "Library is empty.\n";
            return;
        }
        cout << "\n------------------------------------------------------------\n";
        cout << left << setw(5) << "ID" << setw(20) << "Title"
             << setw(20) << "Author" << setw(15) << "Status\n";
        cout << "------------------------------------------------------------\n";
        for (const auto& book : books) {
            book.display();
        }
        cout << "------------------------------------------------------------\n";
    }

    void issueBook(int id) {
        for (auto& book : books) {
            if (book.getId() == id) {
                if (!book.getStatus()) {
                    book.issueBook();
                    cout << "Book issued successfully!\n";
                } else {
                    cout << "Book is already issued.\n";
                }
                return;
            }
        }
        cout << "Book not found.\n";
    }

    void returnBook(int id) {
        for (auto& book : books) {
            // ERROR 2: Look closely at the condition in the if-statement
            if (book.getId() = id) { 
                if (book.getStatus()) {
                    book.returnBook();
                    cout << "Book returned successfully!\n";
                } else {
                    cout << "Book was not issued.\n";
                }
                return;
            }
        }
        cout << "Book not found.\n";
    }
};

// --- Main Execution ---
int main() {
    Library lib;
    int choice, id;
    string title, author;

    while (true) {
        cout << "\n=== Library Management System ===\n";
        cout << "1. Add New Book\n2. Display All Books\n";
        cout << "3. Issue a Book\n4. Return a Book\n5. Exit\n";
        cout << "Enter your choice: ";
        cin >> choice;

        if (choice == 5) {
            cout << "Exiting system. Goodbye!\n";
            break;
        }

        switch (choice) {
            case 1:
                cout << "Enter Book ID: ";
                cin >> id;
                cin.ignore(); 
                cout << "Enter Book Title: ";
                getline(cin, title);
                cout << "Enter Author Name: ";
                getline(cin, author);
                lib.addBook(id, title, author);
                break;
            case 2:
                lib.displayAll();
                break;
            case 3:
                cout << "Enter Book ID to issue: ";
                cin >> id;
                lib.issueBook(id);
                break;
            case 4:
                cout << "Enter Book ID to return: ";
                cin >> id;
                lib.returnBook(id);
                break;
            default:
                cout << "Invalid choice. Please try again.\n";
        }
    }
    return 0;
}

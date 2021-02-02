# Distributed File JSON

**WARNING: This library should not and cannot be used as is.**

This is here for back-up purposes and was in an embedded library within the map editor project mentioned below. This was an experiment to see how I could make dealing with merge conflicts easy for designers, however, my current plan is to now just call `git --no-pager show HEAD:path` and `git --no-pager show MERGE_HEAD:path` for conflicted files and make the tool have the ability to resolve the differences and conflicts within the map files.

## What does it do?

This library allows you to add additional tags to your struct data so that file data can be spread across multiple files.

## Install

```
go get github.com/silbinarywolf/dfjson
```

## Requirements

- Go 1.15

## What is the use-case for this library?

I'm *very very slowly* putting together a 2D game map editor tool in Go and something that I want it to have is good support out of the box for concurrent world editing as I'd like multiple people (possibly non-technical) to be able to make changes to game data while keeping merge conflicts low or easy to resolve.

## Why is godirwalk a dependency?

It's generally [4x faster](https://github.com/karrick/godirwalk#its-faster-than-filepathwalk) than the standard library at walking directories, which is important for this library if we want load times to be reasonable.

## Credits

- [Jonathan Blow](https://www.gamasutra.com/view/news/128846/Indepth_Concurrent_World_Editing_On_The_Cheap.php) for his post on The Witness's Concurrent World Editor.
